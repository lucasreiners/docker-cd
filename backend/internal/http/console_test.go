package handler

import (
	"context"
	"encoding/json"
	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/operations"
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

func TestOperationsImmediateParentPaginationAndLiveCapability(t *testing.T) {
	b := desiredstate.NewBroadcaster()
	parent := b.BeginOperation(context.Background(), "refresh", "manual", "", "")
	other := b.BeginOperation(context.Background(), "refresh", "manual", "", "")
	for i := 0; i < 4; i++ {
		ctx := b.BeginOperation(parent, "deployment", "manual", "nested/web", "")
		b.EndOperation(ctx, "completed", "done")
	}
	child := b.BeginOperation(parent, "deployment", "manual", "nested/web", "")
	b.BeginOperation(child, "deployment", "manual", "nested/web", "") // not an immediate child of parent
	b.BeginOperation(other, "deployment", "manual", "nested/web", "")
	r := NewRouter(config.Config{}, nil, desiredstate.NewStore(), nil, nil, nil, nil, b)
	url := "/api/operations?parentId=" + operations.ID(parent) + "&kind=deployment&status=completed&stack=nested%2Fweb&limit=2"
	ids := map[string]bool{}
	for page := 0; page < 2; page++ {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest("GET", url, nil))
		var payload struct {
			Records    []operations.Record `json:"records"`
			NextCursor string              `json:"nextCursor"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Records) != 2 {
			t.Fatalf("wrong filtered page: %s", response.Body.String())
		}
		for _, record := range payload.Records {
			if ids[record.ID] || record.ParentID != operations.ID(parent) {
				t.Fatal("duplicate/unrelated child")
			}
			ids[record.ID] = true
		}
		if page == 0 {
			if payload.NextCursor == "" {
				t.Fatal("missing next cursor")
			}
			url += "&cursor=" + payload.NextCursor
		} else if payload.NextCursor != "" {
			t.Fatal("unexpected next cursor")
		}
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/api/instance", nil))
	if !strings.Contains(response.Body.String(), `"liveWorkVersion":2`) {
		t.Fatal("implemented work capability missing")
	}
	r = NewRouter(config.Config{}, nil, desiredstate.NewStore(), nil, nil, nil, nil, nil)
	response = httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("GET", "/api/instance", nil))
	if strings.Contains(response.Body.String(), "liveWorkVersion") {
		t.Fatal("nil publisher advertised detailed live work")
	}
}

func TestReviewedActionHTTPRejectsUnsafeScopesAndPolicyBypasses(t *testing.T) {
	for _, tc := range []struct {
		name, action, stack, revision    string
		reconcile, updater, ack, blocked bool
		status                           int
	}{
		{"missing revision", "retry", "deep/web", "", true, true, false, false, 400},
		{"unknown action", "recreate", "deep/web", "current", true, true, false, false, 400},
		{"container mutation", "restart", "deep/web/container", "current", true, true, false, false, 400},
		{"arbitrary stack", "retry", "../unmanaged", "current", true, true, false, false, 404},
		{"reconciliation disabled", "retry", "deep/web", "current", false, true, false, false, 409},
		{"updater disabled", "update", "deep/web", "current", true, false, false, false, 409},
		{"update cannot bypass approval", "update", "deep/web", "current", true, true, true, false, 409},
		{"retry cannot bypass approval", "retry", "deep/web", "current", true, true, true, false, 409},
		{"blocked refresh", "acknowledge", "deep/web", "current", true, true, true, true, 409},
		{"approval exact revision", "acknowledge", "deep/web", "old", true, true, true, false, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := desiredstate.NewStore()
			store.Set(&desiredstate.Snapshot{Revision: "current", RefreshStatus: desiredstate.RefreshStatusCompleted, UpdatesBlocked: tc.blocked, Stacks: []desiredstate.StackRecord{{Path: "deep/web", RequiresAcknowledgement: tc.ack}}})
			runner := &blockingReviewedRunner{started: make(chan context.Context, 1), release: make(chan struct{})}
			defer close(runner.release)
			b := desiredstate.NewBroadcaster()
			router := NewRouter(config.Config{ReconcileEnabled: tc.reconcile, UpdaterEnabled: tc.updater}, nil, store, nil, runner, nil, nil, b)
			body, _ := json.Marshal(map[string]string{"action": tc.action, "stack": tc.stack, "revision": tc.revision})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/api/operations", strings.NewReader(string(body))))
			if response.Code != tc.status {
				t.Fatalf("expected %d got %d: %s", tc.status, response.Code, response.Body.String())
			}
			select {
			case <-runner.started:
				t.Fatal("rejected action reached execution")
			default:
			}
			if len(b.Operations().List()) != 0 {
				t.Fatal("rejected action created an accepted/completed operation")
			}
		})
	}
}

type capturedReviewedCall struct {
	path, revision string
	ack            bool
}

type capturingReviewedRunner struct{ calls chan capturedReviewedCall }

func (r *capturingReviewedRunner) Reconcile(context.Context) []reconcile.ReconciliationRun {
	return nil
}
func (r *capturingReviewedRunner) ReconcileReviewed(_ context.Context, path, revision string, ack bool) reconcile.ReconciliationRun {
	r.calls <- capturedReviewedCall{path, revision, ack}
	// Model the authoritative under-lock rejection after initial acceptance.
	return reconcile.ReconciliationRun{StackPath: path, Result: "failed", Error: "Desired revision changed; review the stack again"}
}

func TestAcceptedActionsKeepExactScopeAndRevisionAndPreserveExecutionRejection(t *testing.T) {
	for _, action := range []string{"retry", "update", "acknowledge"} {
		t.Run(action, func(t *testing.T) {
			store := desiredstate.NewStore()
			store.Set(&desiredstate.Snapshot{Revision: "full-reviewed-revision", RefreshStatus: desiredstate.RefreshStatusCompleted, Stacks: []desiredstate.StackRecord{{Path: "deep/apps/web", RequiresAcknowledgement: action == "acknowledge"}, {Path: "other"}}})
			runner := &capturingReviewedRunner{calls: make(chan capturedReviewedCall, 1)}
			b := desiredstate.NewBroadcaster()
			router := NewRouter(config.Config{ReconcileEnabled: true, UpdaterEnabled: true}, nil, store, nil, runner, nil, nil, b)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest("POST", "/api/operations", strings.NewReader(`{"action":"`+action+`","stack":"deep/apps/web","revision":"full-reviewed-revision"}`)))
			var accepted struct {
				OperationID string `json:"operationId"`
			}
			if response.Code != 202 || json.Unmarshal(response.Body.Bytes(), &accepted) != nil || accepted.OperationID == "" {
				t.Fatalf("missing accepted operation identity: %d %s", response.Code, response.Body.String())
			}
			select {
			case call := <-runner.calls:
				if call.path != "deep/apps/web" || call.revision != "full-reviewed-revision" || call.ack != (action == "acknowledge") {
					t.Fatalf("action scope drifted: %+v", call)
				}
			case <-time.After(time.Second):
				t.Fatal("accepted action never reached runner")
			}
			deadline := time.Now().Add(time.Second)
			for time.Now().Before(deadline) {
				record, ok := b.Operations().Get(accepted.OperationID)
				if ok && record.Status != "running" {
					if record.Status != "failed" || record.Stack != "deep/apps/web" || record.Revision != "full-reviewed-revision" || !strings.Contains(record.Message, "review the stack again") {
						t.Fatalf("execution rejection became success or changed revision: %+v", record)
					}
					return
				}
				time.Sleep(time.Millisecond)
			}
			t.Fatal("rejected execution outcome was not recorded")
		})
	}
}
