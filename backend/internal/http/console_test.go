package handler

import (
	"context"
	"encoding/json"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type blockingReviewedRunner struct {
	started chan context.Context
	release chan struct{}
}

func (r *blockingReviewedRunner) Reconcile(context.Context) []reconcile.ReconciliationRun { return nil }
func (r *blockingReviewedRunner) ReconcileReviewed(ctx context.Context, path, revision string, ack bool) reconcile.ReconciliationRun {
	r.started <- ctx
	<-r.release
	return reconcile.ReconciliationRun{StackPath: path, Result: "success"}
}

func TestStackActionsReturnBeforeCompletionAndRejectStaleOrDuplicateRequests(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "current", RefreshStatus: desiredstate.RefreshStatusCompleted, Stacks: []desiredstate.StackRecord{{Path: "apps/web"}}})
	runner := &blockingReviewedRunner{started: make(chan context.Context, 1), release: make(chan struct{})}
	b := desiredstate.NewBroadcaster()
	r := NewRouter(config.Config{ReconcileEnabled: true}, nil, store, nil, runner, nil, nil, b)
	post := func(revision string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("POST", "/api/operations", strings.NewReader(`{"action":"retry","stack":"apps/web","revision":"`+revision+`"}`)))
		return rec
	}
	if response := post("old"); response.Code != 409 {
		t.Fatalf("stale revision accepted: %d", response.Code)
	}
	response := post("current")
	if response.Code != 202 {
		t.Fatalf("not accepted: %s", response.Body.String())
	}
	select {
	case ctx := <-runner.started:
		if ctx.Err() != nil {
			t.Fatal("job inherited finished HTTP request")
		}
	case <-time.After(time.Second):
		t.Fatal("job did not start")
	}
	if duplicate := post("current"); duplicate.Code != 409 {
		t.Fatal("duplicate action accepted")
	}
	close(runner.release)
	var accepted struct {
		OperationID string `json:"operationId"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		record, _ := b.Operations().Get(accepted.OperationID)
		if record.Status == "completed" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background result was not recorded")
}

func TestInstanceMetadataOmitsCredentialsAndReportsCapabilities(t *testing.T) {
	cfg := config.Config{ProjectName: "test", GitRepoURL: "https://user:supersecret@example.com/repo.git?token=private", GitAccessToken: "access-secret", WebhookSecret: "webhook-secret"}
	r := NewRouter(cfg, nil, desiredstate.NewStore(), nil, nil, nil, nil, desiredstate.NewBroadcaster())
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/instance", nil))
	if recorder.Code != 200 {
		t.Fatal(recorder.Code)
	}
	body := recorder.Body.String()
	for _, secret := range []string{"supersecret", "access-secret", "webhook-secret", "private"} {
		if strings.Contains(body, secret) {
			t.Fatalf("credential leaked: %s", secret)
		}
	}
	var response struct {
		Capabilities struct{ Apply bool }
		Repository   struct{ URL string }
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Capabilities.Apply || response.Repository.URL != "https://example.com/repo.git" {
		t.Fatalf("unexpected metadata: %+v", response)
	}
}

func TestOperationsPaginationAndNestedSource(t *testing.T) {
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "deep/apps/web", ComposeFile: "compose.yaml", Content: []byte("services: {}")}}})
	b := desiredstate.NewBroadcaster()
	for i := 0; i < 3; i++ {
		b.Operations().Start("", "deployment", "manual", "deep/apps/web", "")
	}
	r := NewRouter(config.Config{}, nil, store, nil, nil, nil, nil, b)
	call := func(url string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", url, nil))
		return rec
	}
	response := call("/api/operations?limit=2&stack=deep%2Fapps%2Fweb")
	var page struct {
		Records    []json.RawMessage `json:"records"`
		NextCursor string            `json:"nextCursor"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 || page.NextCursor == "" {
		t.Fatalf("bad page: %+v", page)
	}
	response = call("/api/operations?limit=2&cursor=" + page.NextCursor)
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 1 {
		t.Fatal("cursor duplicated entries")
	}
	if result := call("/api/stack/configuration?path=deep%2Fapps%2Fweb"); result.Code != 200 || !strings.Contains(result.Body.String(), "services: {}") {
		t.Fatal(result.Body.String())
	}
	if result := call("/api/stack/configuration?path=../../etc/passwd"); result.Code != 404 {
		t.Fatal("unknown path accepted")
	}
}
