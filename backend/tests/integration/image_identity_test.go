//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"github.com/lucasreiners/docker-cd/internal/docker"
	handler "github.com/lucasreiners/docker-cd/internal/http"
	"github.com/lucasreiners/docker-cd/internal/reconcile"
	"github.com/lucasreiners/docker-cd/tests/integration/dind"
	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

// Registry manifests and Engine image IDs are observed independently: Docker's
// containerd store may use a manifest digest as Image.ID, not a config digest.
func TestRunningImageIdentityAcrossMutableTag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	fixture, err := dind.NewMutableImage(ctx, h.Env)
	if err != nil {
		t.Fatal(err)
	}
	compose := strings.ReplaceAll(harness.Compose("identity-a"), harness.Image, fixture.Ref)
	if _, err := h.Commit(map[string]*string{"apps/web/compose.yaml": &compose}, "use mutable image"); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(h.Router)
	defer server.Close()
	h.Start()
	waitSynced(t, h, 2)
	api, err := dind.NewSDKClient(h.Env.DockerHost)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	c1 := waitForMutableImageContainer(t, ctx, api, "apps/web", fixture.Initial.ImageID, "")
	listed, err := h.Gate.ComposePs(ctx, "apps-web")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("managed ownership diagnostics: Compose rows=%+v; actual ID=%s stack=%q project=%q service=%q", listed, c1.ID, c1.Config.Labels[reconcile.LabelStackPath], c1.Config.Labels["com.docker.compose.project"], c1.Config.Labels["com.docker.compose.service"])
	inspectIdentityEvidence(t, ctx, api, "initial A", c1.ID, fixture.Initial.ImageID, fixture.Ref)
	a := readIdentityRuntime(t, h.Router, "apps/web")
	assertIdentity(t, a, c1.ID, fixture.Initial.ImageID, fixture.Ref, fixture.Initial.Digest)
	cached := readIdentityRuntime(t, h.Router, "apps/web")
	if !cached.ObservedAt.Equal(*a.ObservedAt) {
		t.Fatal("cache hit invented a new observation timestamp")
	}
	changed, err := fixture.PublishChanged(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if remote, err := fixture.RemoteDigest(ctx); err != nil || remote != changed.Digest {
		t.Fatalf("remote B: %s, %v", remote, err)
	}
	if local, err := fixture.LocalImageID(ctx); err != nil || local != fixture.Initial.ImageID {
		t.Fatalf("local A: %s, %v", local, err)
	}
	// Reconnect is a production invalidation signal, not a fabricated fresh DTO.
	inspectIdentityEvidence(t, ctx, api, "remote B/local A", c1.ID, fixture.Initial.ImageID, fixture.Ref)
	reconnectRuntime(t, ctx, server.URL)
	remoteMoved := readIdentityRuntime(t, h.Router, "apps/web")
	assertIdentity(t, remoteMoved, c1.ID, fixture.Initial.ImageID, fixture.Ref, fixture.Initial.Digest)
	if !remoteMoved.ObservedAt.After(*a.ObservedAt) {
		t.Fatal("reconnect did not reobserve runtime")
	}
	if err := h.Client.PullImages(ctx, []string{fixture.Ref}, nil); err != nil {
		t.Fatal(err)
	}
	if local, err := fixture.LocalImageID(ctx); err != nil || local != changed.ImageID {
		t.Fatalf("pulled local B: %s, %v", local, err)
	}
	// Reconnect storm protection is one second; wait outside that window.
	time.Sleep(1100 * time.Millisecond)
	reconnectRuntime(t, ctx, server.URL)
	pulled := readIdentityRuntime(t, h.Router, "apps/web")
	pulledDigests := inspectIdentityEvidence(t, ctx, api, "pulled B/running A", c1.ID, fixture.Initial.ImageID, fixture.Ref)
	assertIdentityMatchesDaemon(t, "pulled B/running A", pulled, c1.ID, fixture.Initial.ImageID, fixture.Ref, fixture.Initial.Digest, changed.Digest, pulledDigests)
	if !pulled.ObservedAt.After(*remoteMoved.ObservedAt) {
		t.Fatal("post-pull reconnect did not reobserve runtime")
	}
	if current := waitForMutableImageContainer(t, ctx, api, "apps/web", fixture.Initial.ImageID, ""); current.ID != c1.ID {
		t.Fatal("pull replaced C1")
	}
	// Restore only the local A tag so the existing updater observes a real
	// before/after change. Registry B and the running C1 remain untouched.
	runner := &dindRunner{Host: h.Env.DockerHost}
	if output, err := runner.Run(ctx, "docker", "tag", fixture.Initial.ImageID, fixture.Ref); err != nil {
		t.Fatalf("restore local A: %s: %v", output, err)
	}
	restoredDigests := inspectIdentityEvidence(t, ctx, api, "restored local A", c1.ID, fixture.Initial.ImageID, fixture.Ref)
	time.Sleep(1100 * time.Millisecond)
	reconnectRuntime(t, ctx, server.URL)
	restored := readIdentityRuntime(t, h.Router, "apps/web")
	assertIdentityMatchesDaemon(t, "restored local A", restored, c1.ID, fixture.Initial.ImageID, fixture.Ref, fixture.Initial.Digest, changed.Digest, restoredDigests)
	if !restored.ObservedAt.After(*pulled.ObservedAt) {
		t.Fatal("post-tag-restore reconnect did not reobserve runtime")
	}
	work := observeWork(t, ctx, server.URL)
	defer work.Close()
	h.Gate.Arm()
	defer h.Gate.Release()
	w := httptest.NewRecorder()
	h.Router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/update", nil))
	if w.Code != http.StatusOK && w.Code != http.StatusAccepted {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	eventually(t, "changed image reaches managed Compose apply", h.Gate.Waiting)
	var applying desiredstate.WorkRun
	eventually(t, "updater child remains running after pull", func() bool {
		for _, run := range work.runs() {
			if run.Source == "reconcile" && run.Stack == "apps/web" && run.Phase == "applying" && run.Status == "running" {
				applying = run
				return true
			}
		}
		return false
	})
	var schedulerPull desiredstate.WorkRun
	eventually(t, "scheduler pull explicitly completed before child apply", func() bool {
		for _, run := range work.runs() {
			if run.Source == "scheduler" && run.Stack == "apps/web" && run.Status == "completed" {
				schedulerPull = run
				return true
			}
		}
		return false
	})
	if applying.RootOperationID != schedulerPull.RootOperationID {
		t.Fatalf("pull and apply lost ancestry: %+v %+v", schedulerPull, applying)
	}
	for _, op := range work.operations() {
		if op.ID == applying.RootOperationID && op.Status != "running" {
			t.Fatalf("updater ended while Compose is blocked: %+v", op)
		}
	}
	// Explicitly reobserve while Compose is held. A recent legitimate cached
	// observation from before the updater pull must not be mistaken for current
	// daemon repository metadata, nor stamped with a fabricated new timestamp.
	time.Sleep(1100 * time.Millisecond)
	reconnectRuntime(t, ctx, server.URL)
	beforeApply := readIdentityRuntime(t, h.Router, "apps/web")
	beforeApplyDigests := inspectIdentityEvidence(t, ctx, api, "updater pulled B/Compose blocked/C1 still A", c1.ID, fixture.Initial.ImageID, fixture.Ref)
	assertIdentityMatchesDaemon(t, "updater pulled B/Compose blocked/C1 still A", beforeApply, c1.ID, fixture.Initial.ImageID, fixture.Ref, fixture.Initial.Digest, changed.Digest, beforeApplyDigests)
	h.Gate.Release()
	c2 := waitForMutableImageContainer(t, ctx, api, "apps/web", changed.ImageID, c1.ID)
	eventually(t, "Compose deployment explicitly completed", func() bool {
		for _, run := range work.runs() {
			if run.RunID == applying.RunID && run.Status == "completed" {
				return true
			}
		}
		return false
	})
	b := readIdentityRuntime(t, h.Router, "apps/web")
	inspectIdentityEvidence(t, ctx, api, "replacement C2 running B", c2.ID, changed.ImageID, fixture.Ref)
	assertIdentity(t, b, c2.ID, changed.ImageID, fixture.Ref, changed.Digest)
	if !b.ObservedAt.After(*beforeApply.ObservedAt) {
		t.Fatal("deployment terminal did not invalidate runtime")
	}
	assertTerminalWork(t, work, applying.RunID, "completed")
	t.Logf("real identity lifecycle: C1=%s A=%s repositoryA=%s; pulled B=%s repositoryB=%s retained C1; C2=%s reports B; observations %s -> %s", c1.ID, fixture.Initial.ImageID, fixture.Initial.Digest, changed.ImageID, changed.Digest, c2.ID, a.ObservedAt.Format(time.RFC3339Nano), b.ObservedAt.Format(time.RFC3339Nano))
}

func inspectIdentityEvidence(t *testing.T, ctx context.Context, api *dind.AuthenticatedClient, stage, containerID, imageID, reference string) []string {
	t.Helper()
	c, err := api.ContainerInspect(ctx, containerID)
	if err != nil {
		t.Fatalf("%s container inspect: %v", stage, err)
	}
	if c.ID != containerID || c.Image != imageID || c.Config == nil || c.State == nil {
		t.Fatalf("%s unexpected container identity: ID=%s image=%s configMissing=%t stateMissing=%t", stage, c.ID, c.Image, c.Config == nil, c.State == nil)
	}
	if c.Config.Image != reference || !c.State.Running {
		t.Fatalf("%s unexpected container reference/state: Config.Image=%q running=%t", stage, c.Config.Image, c.State.Running)
	}
	im, _, err := api.ImageInspectWithRaw(ctx, c.Image)
	if err != nil {
		t.Fatalf("%s immutable image inspect: %v", stage, err)
	}
	if im.ID != imageID {
		t.Fatalf("%s immutable image inspect ID=%s, want %s", stage, im.ID, imageID)
	}
	t.Logf("daemon evidence [%s]: container=%s running=%t immutableImage=%s Config.Image=%q RepoTags=%v RepoDigests=%v platform=%s/%s/%s", stage, c.ID, c.State.Running, c.Image, c.Config.Image, im.RepoTags, im.RepoDigests, im.Os, im.Architecture, im.Variant)
	return im.RepoDigests
}

// A moved tag can remove the old image's repository association in Docker's
// containerd store. Accept only the two independently verified fixture outcomes:
// exact repository A is still recorded, or the immutable image has no RepoDigests.
// This never derives a repository identity from Image.ID or the current tag.
func assertIdentityMatchesDaemon(t *testing.T, stage string, o handler.RuntimeObservation, containerID, imageID, reference, expectedDigest, forbiddenDigest string, daemonDigests []string) {
	t.Helper()
	row := o.Containers[0]
	i := row.ImageIdentity
	if row.ID == "" || !strings.HasPrefix(containerID, row.ID) || i == nil || i.ImageID != imageID || i.Reference != reference || i.Source != "container_image_inspect" || i.Freshness != "not_checked" || i.Platform == nil {
		t.Fatalf("%s lost immutable running identity/reference: row=%+v identity=%+v", stage, row, i)
	}
	for _, digest := range i.RepositoryDigests {
		if digest.Digest == forbiddenDigest {
			t.Fatalf("%s substituted newer B's digest into running A: %+v", stage, i)
		}
	}
	if len(daemonDigests) == 0 {
		if i.Status != "unavailable" || i.Reason != "not_recorded" || len(i.RepositoryDigests) != 0 || i.PreferredRepositoryDigest != nil {
			t.Fatalf("%s daemon has no RepoDigests but API invented/misreported repository identity: %+v", stage, i)
		}
		t.Logf("API matches daemon [%s]: C1 unchanged, immutable A=%s reference=%q; daemon lost repository association, API status=unavailable reason=not_recorded with no preferred digest (not B)", stage, i.ImageID, i.Reference)
		return
	}
	repository := strings.TrimSuffix(reference, ":current")
	if len(daemonDigests) != 1 || daemonDigests[0] != repository+"@"+expectedDigest {
		t.Fatalf("%s unexpected daemon repository metadata: %v; want only A=%s@%s or no association", stage, daemonDigests, repository, expectedDigest)
	}
	assertIdentity(t, o, containerID, imageID, reference, expectedDigest)
	if i.Reason != "" || len(i.RepositoryDigests) != 1 || i.RepositoryDigests[0].Repository != repository || i.RepositoryDigests[0].Digest != expectedDigest || i.RepositoryDigests[0].Kind != "unknown" {
		t.Fatalf("%s API omitted/changed daemon's retained A association: %+v", stage, i)
	}
	t.Logf("API matches daemon [%s]: C1 unchanged, immutable A=%s reference=%q; available repository A=%s retained", stage, i.ImageID, i.Reference, expectedDigest)
}

func TestLocalCommittedImageIdentityWithoutRepositoryDigest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	env := dind.StartT(t)
	runner := &dindRunner{Host: env.DockerHost}
	api, err := dind.NewSDKClient(env.DockerHost)
	if err != nil {
		t.Fatal(err)
	}
	defer api.Close()
	if output, err := runner.Run(ctx, "docker", "pull", harness.Image); err != nil {
		t.Fatalf("warm local base: %s: %v", output, err)
	}
	seed, err := runner.Run(ctx, "docker", "create", harness.Image, "sleep", "3600")
	if err != nil {
		t.Fatal(err)
	}
	// Commit without a repository tag. Docker 29's containerd image store may
	// synthesize a RepoDigest even for a named local commit that was never pushed.
	// An untagged committed image is the actual no-repository-identity case.
	output, err := runner.Run(ctx, "docker", "commit", "--change", "LABEL identity.local=true", strings.TrimSpace(string(seed)))
	if err != nil {
		t.Fatalf("commit: %s: %v", output, err)
	}
	localRef := strings.TrimSpace(string(output))
	local, _, err := api.ImageInspectWithRaw(ctx, localRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(local.RepoDigests) != 0 {
		t.Fatalf("local fixture unexpectedly has RepoDigests: %v", local.RepoDigests)
	}
	t.Logf("actual untagged committed image=%s, RepoDigests=%v (never pushed)", local.ID, local.RepoDigests)
	content := []byte(fmt.Sprintf("services:\n  worker:\n    image: %s\n    pull_policy: never\n    command: [sleep, '3600']\n", localRef))
	store := desiredstate.NewStore()
	store.Set(&desiredstate.Snapshot{Revision: "local", RefreshStatus: desiredstate.RefreshStatusCompleted, Stacks: []desiredstate.StackRecord{{Path: "local-only", ComposeFile: "compose.yaml", Content: content, ComposeHash: desiredstate.ComposeHash(content), Status: desiredstate.StackSyncMissing}}})
	client := docker.NewClient(api)
	r := newDindReconciler(store, reconcile.DefaultPolicy(), reconcile.NewDockerComposeRunner(runner, env.DockerHost), reconcile.NewDockerContainerInspector(client), reconcile.NewAckStore(), "")
	if run := r.ReconcileStack(ctx, "local-only"); run.Result != "success" {
		t.Fatalf("local apply: %+v", run)
	}
	router := handler.NewRouter(config.Config{}, nil, store, nil, r, nil, api)
	observation := readIdentityRuntime(t, router, "local-only")
	identity := observation.Containers[0].ImageIdentity
	if identity == nil || identity.ImageID != local.ID || identity.Status != "unavailable" || identity.Reason != "not_recorded" || len(identity.RepositoryDigests) != 0 || identity.PreferredRepositoryDigest != nil || identity.Freshness != "not_checked" {
		t.Fatalf("local identity: %+v", identity)
	}
	t.Logf("actual local commit image=%s, no RepoDigests, status=%s reason=%s", local.ID, identity.Status, identity.Reason)
}

func readIdentityRuntime(t *testing.T, router http.Handler, stack string) handler.RuntimeObservation {
	t.Helper()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/runtime", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("runtime status %d: %s", w.Code, w.Body.String())
	}
	var body map[string]handler.RuntimeObservation
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	o, ok := body[stack]
	if !ok || o.Error != nil || o.ObservedAt == nil || len(o.Containers) != 1 {
		t.Fatalf("runtime observation: %+v", o)
	}
	return o
}

func assertIdentity(t *testing.T, o handler.RuntimeObservation, containerID, imageID, reference, digest string) {
	t.Helper()
	row := o.Containers[0]
	i := row.ImageIdentity
	if row.ID == "" || !strings.HasPrefix(containerID, row.ID) || i == nil || i.Status != "available" || i.ImageID != imageID || i.Reference != reference || i.Freshness != "not_checked" || i.PreferredRepositoryDigest == nil || i.PreferredRepositoryDigest.Digest != digest || i.PreferredRepositoryDigest.Kind != "unknown" || i.Platform == nil {
		t.Fatalf("identity for container %s, image %s, digest %s: row=%+v identity=%+v", containerID, imageID, digest, row, i)
	}
}

func reconnectRuntime(t *testing.T, ctx context.Context, base string) {
	t.Helper()
	requestCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, base+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reconnect status %d", resp.StatusCode)
	}
}
