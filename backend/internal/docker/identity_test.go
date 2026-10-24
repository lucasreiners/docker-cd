package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/errdefs"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

type identitySDK struct {
	container              container.InspectResponse
	image                  image.InspectResponse
	containerErr, imageErr error
	containers, images     []string
}

func TestIdentityHostileMetadataIsUnavailableNotPartial(t *testing.T) {
	for _, kind := range []string{"digest-count", "digest-length", "platform"} {
		t.Run(kind, func(t *testing.T) {
			sdk, row := identityFixture()
			switch kind {
			case "digest-count":
				sdk.image.RepoDigests = make([]string, 100000)
				for i := range sdk.image.RepoDigests {
					sdk.image.RepoDigests[i] = "nginx@sha256:" + strings.Repeat("c", 64)
				}
			case "digest-length":
				sdk.image.RepoDigests = []string{strings.Repeat("x", 1024*1024)}
			case "platform":
				sdk.image.Os = strings.Repeat("x", 1024*1024)
			}
			r := NewIdentityReader(sdk)
			got := r.Resolve(context.Background(), "apps/web", row)
			if got.Status != "unavailable" || got.Reason != "inspection_failed" || len(got.RepositoryDigests) != 0 || got.PreferredRepositoryDigest != nil || got.Platform != nil {
				t.Fatalf("oversized inspect became partial identity: %+v", got)
			}
			if r.metadataBytes > identityCacheBudget {
				t.Fatal("cache retained oversized metadata")
			}
		})
	}
}

func TestIdentityMetadataByteBudget(t *testing.T) {
	sdk, row := identityFixture()
	sdk.image.RepoDigests = nil
	for i := 0; i < IdentityDigestLimit; i++ {
		sdk.image.RepoDigests = append(sdk.image.RepoDigests, fmt.Sprintf("registry.example/%s%d@sha256:%s", strings.Repeat("a", 150), i, strings.Repeat("c", 64)))
	}
	r := NewIdentityReader(sdk)
	for i := 0; i < 1100; i++ {
		id := fmt.Sprintf("sha256:%064x", i)
		sdk.container.Image, sdk.image.ID = id, id
		got := r.Resolve(context.Background(), "apps/web", row)
		if len(got.RepositoryDigests) != IdentityDigestLimit {
			t.Fatal("memoization cap truncated reported identity")
		}
	}
	if r.metadataBytes > identityCacheBudget || len(r.images) >= identityCacheLimit || r.metadataBytes < identityCacheBudget-10000 {
		t.Fatalf("cache byte cap not exercised: bytes=%d images=%d", r.metadataBytes, len(r.images))
	}
}

func (s *identitySDK) ContainerInspect(_ context.Context, id string) (container.InspectResponse, error) {
	s.containers = append(s.containers, id)
	return s.container, s.containerErr
}
func (s *identitySDK) ImageInspectWithRaw(_ context.Context, id string) (image.InspectResponse, []byte, error) {
	s.images = append(s.images, id)
	return s.image, []byte(`{"Env":["SECRET=raw-secret"]}`), s.imageErr
}

func identityFixture() (*identitySDK, desiredstate.ContainerInfo) {
	row := desiredstate.ContainerInfo{FullID: strings.Repeat("a", 64), ID: strings.Repeat("a", 12), ComposeProject: "apps-web", Service: "web", Image: "nginx:stable"}
	id := "sha256:" + strings.Repeat("b", 64)
	sdk := &identitySDK{
		container: container.InspectResponse{ContainerJSONBase: &container.ContainerJSONBase{ID: row.FullID, Image: id}, Config: &container.Config{Image: row.Image, Env: []string{"SECRET=env-secret"}, Labels: map[string]string{"com.docker-cd.stack.path": "apps/web", "com.docker.compose.project": "apps-web", "com.docker.compose.service": "web", "secret": "label-secret"}}},
		image:     image.InspectResponse{ID: id, Os: "linux", Architecture: "arm", Variant: "v7", RepoDigests: []string{"nginx@sha256:" + strings.Repeat("c", 64)}},
	}
	return sdk, row
}

func TestIdentityAnchorsRunningContainerNotMovedTag(t *testing.T) {
	sdk, row := identityFixture()
	// A newer tag resolution could point elsewhere. No tag resolution method is
	// available on this inspector, and the immutable old image must be requested.
	r := NewIdentityReader(sdk)
	identity := r.Resolve(context.Background(), "apps/web", row)
	if identity.Status != "available" || identity.ImageID != sdk.container.Image || len(sdk.images) != 1 || sdk.images[0] != sdk.container.Image || sdk.containers[0] != row.FullID {
		t.Fatalf("not anchored to old running image: %+v calls=%v", identity, sdk.images)
	}
	if identity.PreferredRepositoryDigest.Repository != "docker.io/library/nginx" || identity.PreferredRepositoryDigest.Kind != "unknown" || identity.Freshness != "not_checked" || identity.Platform.Variant != "v7" {
		t.Fatalf("wrong metadata: %+v", identity)
	}
	r.Resolve(context.Background(), "apps/web", row)
	if len(sdk.images) != 1 || len(sdk.containers) != 2 {
		t.Fatal("must deduplicate immutable image inspect, not container inspection")
	}
	NewIdentityReader(sdk).Resolve(context.Background(), "apps/web", row)
	if len(sdk.images) != 2 {
		t.Fatal("image metadata retained across cycles")
	}
	encoded, _ := json.Marshal(identity)
	for _, forbidden := range []string{"env-secret", "label-secret", "raw-secret", "Env", "Config", "Labels", "RepoTags"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("inspect data escaped: %s", encoded)
		}
	}
}

func TestRepositoryIdentitySelection(t *testing.T) {
	d1, d2 := "sha256:"+strings.Repeat("c", 64), "sha256:"+strings.Repeat("d", 64)
	for _, tc := range []struct {
		name, configured string
		digests          []string
		status, reason   string
		count            int
	}{
		{"local build", "nginx:stable", nil, "unavailable", "not_recorded", 0},
		{"one normalized match", "nginx:stable", []string{"docker.io/library/nginx@" + d1, "nginx@" + d1}, "available", "", 1},
		{"multiple repositories", "nginx:stable", []string{"other.example/nginx@" + d1, "nginx@" + d2}, "available", "", 2},
		{"multiple matches", "nginx:stable", []string{"nginx@" + d1, "nginx@" + d2}, "ambiguous", "repository_ambiguous", 2},
		{"wrong repository", "nginx:stable", []string{"other.example/nginx@" + d1}, "unavailable", "not_recorded", 1},
		{"registry port", "localhost:5000/team/web:dev", []string{"localhost:5000/team/web@" + d1}, "available", "", 1},
		{"configured digest", "nginx@" + d1, []string{"nginx@" + d1}, "available", "", 1},
		{"invalid digest is not identity", "nginx:stable", []string{"nginx@sha256:short", "nginx:stable"}, "unavailable", "not_recorded", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sdk, row := identityFixture()
			sdk.container.Config.Image = tc.configured
			sdk.image.RepoDigests = tc.digests
			got := NewIdentityReader(sdk).Resolve(context.Background(), "apps/web", row)
			if got.Status != tc.status || got.Reason != tc.reason || len(got.RepositoryDigests) != tc.count {
				t.Fatalf("unexpected: %+v", got)
			}
			if (got.PreferredRepositoryDigest != nil) != (tc.status == "available") {
				t.Fatal("arbitrary preferred identity")
			}
		})
	}
}

func TestIdentityUnavailableIsSanitizedAndScoped(t *testing.T) {
	for _, tc := range []struct {
		name, reason               string
		change                     func(*identitySDK, *desiredstate.ContainerInfo)
		containerCalls, imageCalls int
	}{
		{"short id", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) { r.FullID = r.ID }, 0, 0},
		{"name input", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) { r.FullID = "web" }, 0, 0},
		{"no project", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) { r.ComposeProject = "" }, 0, 0},
		{"wrong stack", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.container.Config.Labels["com.docker-cd.stack.path"] = "other"
		}, 1, 0},
		{"wrong project", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.container.Config.Labels["com.docker.compose.project"] = "other"
		}, 1, 0},
		{"wrong service", "ownership_unverified", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.container.Config.Labels["com.docker.compose.service"] = "other"
		}, 1, 0},
		{"disappeared", "container_replaced", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.containerErr = errdefs.NotFound(errors.New("credential-secret"))
		}, 1, 0},
		{"replacement id", "container_replaced", func(s *identitySDK, r *desiredstate.ContainerInfo) { s.container.ID = strings.Repeat("f", 64) }, 1, 0},
		{"missing image", "image_missing", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.imageErr = errdefs.NotFound(errors.New("credential-secret"))
		}, 1, 1},
		{"denied", "inspection_denied", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.containerErr = errdefs.Forbidden(errors.New("credential-secret"))
		}, 1, 0},
		{"unsupported", "unsupported", func(s *identitySDK, r *desiredstate.ContainerInfo) {
			s.containerErr = errdefs.NotImplemented(errors.New("credential-secret"))
		}, 1, 0},
		{"failure", "inspection_failed", func(s *identitySDK, r *desiredstate.ContainerInfo) { s.imageErr = errors.New("credential-secret") }, 1, 1},
		{"mutable image input", "inspection_failed", func(s *identitySDK, r *desiredstate.ContainerInfo) { s.container.Image = "nginx:stable" }, 1, 0},
		{"image mismatch", "inspection_failed", func(s *identitySDK, r *desiredstate.ContainerInfo) { s.image.ID = "sha256:" + strings.Repeat("e", 64) }, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sdk, row := identityFixture()
			tc.change(sdk, &row)
			got := NewIdentityReader(sdk).Resolve(context.Background(), "apps/web", row)
			if got.Status != "unavailable" || got.Reason != tc.reason || len(sdk.containers) != tc.containerCalls || len(sdk.images) != tc.imageCalls {
				t.Fatalf("unsafe result: %+v calls=%v/%v", got, sdk.containers, sdk.images)
			}
			encoded, _ := json.Marshal(got)
			if strings.Contains(string(encoded), "credential-secret") {
				t.Fatal("raw error leaked")
			}
			if tc.imageCalls == 0 && got.ImageID != "" {
				t.Fatal("unverified image ID leaked")
			}
		})
	}
	sdk, row := identityFixture()
	_ = sdk
	if got := NewIdentityReader(nil).Resolve(context.Background(), "apps/web", row); got.Reason != "unsupported" {
		t.Fatal(got)
	}
}

func TestIdentityCacheBound(t *testing.T) {
	sdk, row := identityFixture()
	r := NewIdentityReader(sdk)
	for i := 0; i < 1100; i++ {
		// Valid unique immutable IDs; only safe metadata may be memoized.
		id := strings.Repeat("0", 64)
		b := []byte(id)
		b[60] = "0123456789abcdef"[(i>>12)&15]
		b[61] = "0123456789abcdef"[(i>>8)&15]
		b[62] = "0123456789abcdef"[(i>>4)&15]
		b[63] = "0123456789abcdef"[i&15]
		sdk.container.Image = "sha256:" + string(b)
		sdk.image.ID = sdk.container.Image
		r.Resolve(context.Background(), "apps/web", row)
	}
	if len(r.images) != 1024 {
		t.Fatal("unbounded image cache", len(r.images))
	}
}

type cancelledIdentitySDK struct{ imageCalls int }

func (s *cancelledIdentitySDK) ContainerInspect(ctx context.Context, _ string) (container.InspectResponse, error) {
	<-ctx.Done()
	return container.InspectResponse{}, ctx.Err()
}
func (s *cancelledIdentitySDK) ImageInspectWithRaw(context.Context, string) (image.InspectResponse, []byte, error) {
	s.imageCalls++
	return image.InspectResponse{}, nil, nil
}

func TestIdentityCancellationAndUnknownPlatform(t *testing.T) {
	_, row := identityFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sdk := &cancelledIdentitySDK{}
	got := NewIdentityReader(sdk).Resolve(ctx, "apps/web", row)
	if got.Reason != "inspection_failed" || sdk.imageCalls != 0 {
		t.Fatal("cancelled inspection continued", got)
	}
	fixture, row := identityFixture()
	fixture.image.Os = ""
	got = NewIdentityReader(fixture).Resolve(context.Background(), "apps/web", row)
	if got.Status != "available" || got.Platform != nil || got.PreferredRepositoryDigest.Kind != "unknown" {
		t.Fatal("invented platform/kind", got)
	}
}
