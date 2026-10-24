//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/errdefs"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

// Issue #30 concerns removing one service, not deleting the whole stack file.
// Inspect the real daemon with All=true: hiding or stopping an orphan is not removal.
func TestGitOpsRemovesServiceFromExistingCompose(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	api, err := dind.NewSDKClient(h.Env.DockerHost)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	retained := harness.Compose("web-v1")
	extra := fmt.Sprintf("  retired:\n    image: %s\n    container_name: retired-service\n    command: [sleep, '3600']\n    stop_grace_period: 1s\n    volumes: [retired-data:/retired]\n  retired-stopped:\n    image: %s\n    container_name: retired-stopped-service\n    command: [sleep, '3600']\n    stop_grace_period: 1s\n", harness.Image, harness.Image)
	// Keep the volume definition after removing its service: service removal
	// must never imply deleting persistent application data.
	retained = strings.Replace(retained, "  data: {}\n", "  data: {}\n  retired-data: {}\n", 1)
	initial := strings.Replace(retained, "volumes:\n", extra+"volumes:\n", 1)
	initialRevision, err := h.Commit(map[string]*string{"apps/web/compose.yaml": &initial}, "deploy services for issue 30")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Router)
	defer server.Close()
	h.Start()
	waitRemovalDeployment(t, h, initialRevision, 3)
	list, err := api.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	var retiredID, stoppedID, unrelatedID string
	for _, c := range list {
		if c.Labels[reconcile.LabelStackPath] == "apps/web" && c.Labels["com.docker.compose.service"] == "retired" {
			retiredID = c.ID
		}
		if c.Labels[reconcile.LabelStackPath] == "apps/web" && c.Labels["com.docker.compose.service"] == "retired-stopped" {
			stoppedID = c.ID
		}
		if c.Labels[reconcile.LabelStackPath] == "infra/cache" {
			unrelatedID = c.ID
		}
	}
	if retiredID == "" || stoppedID == "" || unrelatedID == "" {
		t.Fatalf("fixture did not create retired and unrelated services: %v", list)
	}
	if err := api.ContainerStop(ctx, stoppedID, container.StopOptions{}); err != nil {
		t.Fatal(err)
	}
	if c, err := api.ContainerInspect(ctx, stoppedID); err != nil || c.State.Running {
		t.Fatalf("stopped-service fixture is not stopped: %v", err)
	}
	runner := &dindRunner{Host: h.Env.DockerHost}
	if output, err := runner.Run(ctx, "docker", "exec", retiredID, "sh", "-c", "printf issue-30-persistent > /retired/marker"); err != nil {
		t.Fatalf("write fixture volume data: %s: %v", output, err)
	}
	// Record the actual Docker volume identity; it must survive reconciliation.
	beforeVolume, err := api.VolumeInspect(ctx, "apps-web_retired-data")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := h.Commit(map[string]*string{"apps/web/compose.yaml": &retained}, "remove retired service but keep stack")
	if err != nil {
		t.Fatal(err)
	}
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	waitRemovalDeployment(t, h, revision, -1)
	remaining, err := api.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project=apps-web"))})
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].Labels["com.docker.compose.service"] != "worker" || remaining[0].State != "running" {
		var services []string
		for _, c := range remaining {
			services = append(services, c.Labels["com.docker.compose.service"]+":"+c.State)
		}
		t.Fatalf("issue #30: removed service remains on daemon after synced Git revision: %v (retired ID %s)", services, retiredID)
	}
	if _, err := api.ContainerInspect(ctx, retiredID); !errdefs.IsNotFound(err) {
		t.Fatalf("removed container must no longer exist, got %v", err)
	}
	if _, err := api.ContainerInspect(ctx, stoppedID); !errdefs.IsNotFound(err) {
		t.Fatalf("stopped orphan must no longer exist, got %v", err)
	}
	if c, err := api.ContainerInspect(ctx, unrelatedID); err != nil || !c.State.Running {
		t.Fatalf("unrelated stack changed: %v", err)
	}
	afterVolume, err := api.VolumeInspect(ctx, beforeVolume.Name)
	if err != nil || !reflect.DeepEqual(beforeVolume, afterVolume) {
		t.Fatalf("service removal changed persistent volume: %v", err)
	}
	if output, err := runner.Run(ctx, "docker", "run", "--rm", "-v", beforeVolume.Name+":/retired:ro", harness.Image, "cat", "/retired/marker"); err != nil || string(output) != "issue-30-persistent" {
		t.Fatalf("service removal lost volume contents: %q: %v", output, err)
	}
	waitRemovalDeployment(t, h, revision, 1)
	for _, endpoint := range []string{"/api/runtime", "/api/stacks/containers/apps/web"} {
		w := httptest.NewRecorder()
		h.Router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, endpoint, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", endpoint, w.Code, w.Body.String())
		}
		var rows []desiredstate.ContainerInfo
		if endpoint == "/api/runtime" {
			var observations map[string]struct {
				Containers []desiredstate.ContainerInfo `json:"containers"`
				Error      *string                      `json:"error"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &observations); err != nil {
				t.Fatal(err)
			}
			rows = observations["apps/web"].Containers
			if observations["apps/web"].Error != nil {
				t.Fatal("runtime observation failed", *observations["apps/web"].Error)
			}
		} else if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		if len(rows) != 1 || rows[0].Service != "worker" {
			t.Fatalf("%s retained removed service: %+v", endpoint, rows)
		}
	}
	// A duplicate delivery must not recreate the surviving service or resurrect
	// either orphan after cleanup.
	refreshedAt := h.Store.Get().RefreshedAt
	postWebhook(t, server.URL, revision, true, http.StatusOK)
	eventually(t, "duplicate service-removal refresh finishes", func() bool {
		return !h.Queue.IsRunning() && h.Store.Get().RefreshedAt.After(refreshedAt)
	})
	waitRemovalDeployment(t, h, revision, 1)
	afterDuplicate, err := api.ContainerList(ctx, container.ListOptions{All: true, Filters: filters.NewArgs(filters.Arg("label", "com.docker.compose.project=apps-web"))})
	if err != nil || len(afterDuplicate) != 1 || afterDuplicate[0].ID != remaining[0].ID {
		t.Fatalf("duplicate delivery changed surviving containers: %+v %v", afterDuplicate, err)
	}
	t.Logf("issue #30: running and stopped services removed from daemon, API and runtime; retained worker=%s; unrelated stack=%s unchanged; named volume data preserved", remaining[0].ID, unrelatedID)
}

func waitRemovalDeployment(t *testing.T, h *harness.Harness, revision string, containers int) {
	t.Helper()
	eventually(t, "service-removal Git revision is applied", func() bool {
		if h.Queue.IsRunning() {
			return false
		}
		for _, s := range h.Store.GetStacks() {
			if s.Path == "apps/web" {
				return s.Status == desiredstate.StackSyncSynced && s.SyncedRevision == revision && (containers < 0 || s.ContainersTotal == containers)
			}
		}
		return false
	})
}
