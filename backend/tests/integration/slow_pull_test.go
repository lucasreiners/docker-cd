//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/refresh"
	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

func TestSlowPullLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	h.SlowPull.Arm(false)
	h.Start()
	eventually(t, "slow pull starts", h.SlowPull.Active)
	assertSyncing := func() {
		t.Helper()
		for _, s := range h.Store.GetStacks() {
			if s.Path == "apps/web" && (s.Status != desiredstate.StackSyncSyncing || s.LastSyncError != "") {
				t.Fatalf("long pull incorrectly failed: %+v", s)
			}
		}
	}
	// Repeated scheduled refreshes must not kill an in-flight image pull.
	for range 3 {
		h.Refresh.RequestRefresh(refresh.TriggerPeriodic)
		time.Sleep(600 * time.Millisecond)
		assertSyncing()
	}
	for _, phase := range []string{"Downloading", "Extracting", "Extracting"} {
		h.SlowPull.Advance()
		time.Sleep(600 * time.Millisecond)
		assertSyncing()
		sub, initial := h.Broadcaster.SubscribeWithSnapshot(h.Store)
		h.Broadcaster.Unsubscribe(sub)
		var snapshot struct {
			Progress []struct {
				LayerID string `json:"layerId"`
				Status  string `json:"status"`
			} `json:"progress"`
		}
		if err := json.Unmarshal([]byte(initial[2].Data), &snapshot); err != nil {
			t.Fatal(err)
		}
		layers := map[string]string{}
		for _, p := range snapshot.Progress {
			if p.LayerID != "" {
				layers[p.LayerID] = p.Status
			}
		}
		if len(layers) != 2 {
			t.Fatalf("reconnect lost layers: %s", initial[2].Data)
		}
		if layers["layer-a"] != phase && layers["layer-b"] != phase {
			t.Fatalf("missing %s phase: %v", phase, layers)
		}
	}
	h.SlowPull.Advance()
	waitSynced(t, h, 2)
	// A real stream error should fail the initial deployment, then recover on retry.
	newStack := harness.Compose("slow-new")
	if _, err := h.Commit(map[string]*string{"aaa/new/compose.yaml": &newStack}, "add slow stack"); err != nil {
		t.Fatal(err)
	}
	h.SlowPull.Arm(true)
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	eventually(t, "failure fixture starts", h.SlowPull.Active)
	h.SlowPull.Advance()
	h.SlowPull.Advance()
	eventually(t, "pull error ends deployment", func() bool { return !h.Queue.IsRunning() })
	for _, s := range h.Store.GetStacks() {
		if s.Path == "aaa/new" && (s.Status != desiredstate.StackSyncFailed || s.LastSyncError == "") {
			t.Fatalf("pull failure not reported: %+v", s)
		}
	}
	h.Refresh.RequestRefresh(refresh.TriggerManual)
	waitSynced(t, h, 3)
}
