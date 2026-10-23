//go:build integration

package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

func TestGitOpsLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	server := httptest.NewServer(h.Router)
	defer server.Close()
	work := observeWork(t, ctx, server.URL)
	defer work.Close()
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	req, _ := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/api/events", nil)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var mu sync.Mutex
	var stream []string
	go func() {
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			mu.Lock()
			stream = append(stream, scanner.Text())
			mu.Unlock()
		}
	}()
	h.Gate.Arm()
	h.Start()
	eventually(t, "initial deployment reaches Compose", h.Gate.Waiting)
	eventually(t, "initial syncing state arrives over HTTP SSE", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(strings.Join(stream, "\n"), `"status":"syncing"`)
	})
	h.Gate.Release()
	waitSynced(t, h, 2)
	initial := runtimeIDs(t, h)
	if len(initial) != 2 {
		t.Fatalf("expected real containers for two nested stacks: %v", initial)
	}

	// Reject forged webhooks even when the origin has a newer commit.
	changed := harness.Compose("web-v2")
	added := harness.Compose("worker-v1")
	revision, err := h.Commit(map[string]*string{"apps/web/compose.yaml": &changed, "deep/nested/worker/compose.yaml": &added}, "update web and discover worker")
	if err != nil {
		t.Fatal(err)
	}
	postWebhook(t, server.URL, revision, false, http.StatusUnauthorized)
	if h.Store.Get().Revision != h.Revision {
		t.Fatal("invalid webhook changed desired state")
	}

	h.Gate.Arm()
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	eventually(t, "webhook update reaches Compose", h.Gate.Waiting)
	for _, stack := range h.Store.GetStacks() {
		if stack.Path == "apps/web" && (stack.SyncedRevision != h.Revision || stack.ContainersTotal != 1) {
			t.Fatalf("refresh lost last deployed metadata: %+v", stack)
		}
	}
	h.Gate.Release()
	waitSynced(t, h, 3)
	updated := runtimeIDs(t, h)
	if updated["apps/web"] == initial["apps/web"] {
		t.Fatal("changed stack was not recreated")
	}
	if updated["infra/cache"] != initial["infra/cache"] {
		t.Fatal("unchanged stack was recreated")
	}
	if updated["deep/nested/worker"] == "" {
		t.Fatal("nested new stack was not deployed")
	}

	// Duplicate deliveries are idempotent and leave runtime container IDs alone.
	refreshedAt := h.Store.Get().RefreshedAt
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	eventually(t, "duplicate refresh completes", func() bool {
		s := h.Store.Get()
		return s.RefreshedAt.After(refreshedAt) && s.RefreshStatus == desiredstate.RefreshStatusCompleted
	})
	waitSynced(t, h, 3)
	if !reflect.DeepEqual(runtimeIDs(t, h), updated) {
		t.Fatal("duplicate webhook recreated containers")
	}
	labels, err := reconcile.NewDockerContainerInspector(h.Client).GetStackLabels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if labels["apps/web"].DesiredRevision != revision || labels["infra/cache"].DesiredRevision != h.Revision {
		t.Fatalf("runtime labels do not identify each stack's deployed revision: %+v", labels)
	}

	// Fail a real Compose validation, then fix it through Git and webhook.
	broken := "services:\n  worker:\n    image: " + harness.Image + "\n    definitely_invalid_property: true\n"
	revision, err = h.Commit(map[string]*string{"apps/web/compose.yaml": &broken}, "broken compose")
	if err != nil {
		t.Fatal(err)
	}
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	eventually(t, "failed deployment is observable", func() bool {
		for _, s := range h.Store.GetStacks() {
			if s.Path == "apps/web" {
				return s.Status == desiredstate.StackSyncFailed && s.LastSyncError != ""
			}
		}
		return false
	})
	eventually(t, "failed refresh cycle finishes", func() bool { return !h.Queue.IsRunning() })
	var failedApply desiredstate.WorkRun
	eventually(t, "Compose failure has explicit work terminal", func() bool {
		for _, run := range work.runs() {
			if run.Source == "reconcile" && run.Stack == "apps/web" && run.Status == "failed" {
				failedApply = run
				return true
			}
		}
		return false
	})
	assertTerminalWork(t, work, failedApply.RunID, "failed")
	for _, run := range work.runs() {
		if run.RunID == failedApply.RunID && run.Status == "completed" {
			t.Fatal("Compose failure also published success")
		}
	}
	t.Logf("real invalid Compose failure explicitly reported over HTTP SSE: run=%s status=%s", failedApply.RunID, failedApply.Status)
	revision, err = h.Commit(map[string]*string{"apps/web/compose.yaml": &changed}, "recover compose")
	if err != nil {
		t.Fatal(err)
	}
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	waitSynced(t, h, 3)
	for _, s := range h.Store.GetStacks() {
		if s.LastSyncError != "" {
			t.Fatalf("stale error after recovery: %+v", s)
		}
	}

	// Removal policy deletes runtime containers, retaining persistent volumes.
	revision, err = h.Commit(map[string]*string{"deep/nested/worker/compose.yaml": nil}, "remove worker")
	if err != nil {
		t.Fatal(err)
	}
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	waitSynced(t, h, 2)
	eventually(t, "deleted stack removed from daemon", func() bool { return runtimeIDs(t, h)["deep/nested/worker"] == "" })
	eventually(t, "removal cycle finishes", func() bool { return !h.Queue.IsRunning() })
	runner := &dindRunner{Host: h.Env.DockerHost}
	if output, err := runner.Run(ctx, "docker", "volume", "inspect", "deep-nested-worker_data"); err != nil {
		t.Fatalf("stack removal destroyed its persistent volume: %s: %v", output, err)
	}
	eventually(t, "final synced event reaches UI stream", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return strings.Contains(strings.Join(stream, "\n"), `"status":"synced"`) && strings.Contains(strings.Join(stream, "\n"), `"type":"stack_success"`)
	})
}

func eventually(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("timed out: %s", description)
		case <-tick.C:
		}
	}
}

func waitSynced(t *testing.T, h *harness.Harness, count int) {
	t.Helper()
	eventually(t, fmt.Sprintf("%d stacks synced with containers", count), func() bool {
		if h.Queue.IsRunning() {
			return false
		}
		stacks := h.Store.GetStacks()
		if len(stacks) != count {
			return false
		}
		for _, s := range stacks {
			if s.Status != desiredstate.StackSyncSynced || s.ContainersRunning != 1 {
				return false
			}
		}
		return true
	})
}

func runtimeIDs(t *testing.T, h *harness.Harness) map[string]string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	containers, err := h.Client.ListContainersWithLabel(ctx, reconcile.LabelStackPath)
	if err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]string)
	for _, c := range containers {
		ids[c.Labels[reconcile.LabelStackPath]] = c.ContainerID
	}
	return ids
}

func postWebhook(t *testing.T, base, revision string, valid bool, expected int) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{"ref": "refs/heads/main", "after": revision, "repository": map[string]string{"full_name": "test/fixture"}})
	req, _ := http.NewRequest("POST", base+"/api/webhook", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", harness.Signature(body))
	if !valid {
		req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != expected {
		t.Fatalf("webhook status: got %d want %d", resp.StatusCode, expected)
	}
}
