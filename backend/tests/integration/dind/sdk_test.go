package dind

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	"github.com/docker/docker/client"
)

// These tests inject a dummy resolver: no config reads or user helper execution.
func TestAuthenticatedPullRegistryRouting(t *testing.T) {
	for _, tc := range []struct {
		ref, server string
	}{
		{"busybox:latest", "https://index.docker.io/v1/"},
		{"docker.io/library/busybox:latest", "https://index.docker.io/v1/"},
		{"index.docker.io/library/busybox:latest", "https://index.docker.io/v1/"},
		{"registry-1.docker.io/library/busybox:latest", "https://index.docker.io/v1/"},
		{"private.example/team/app:latest", "private.example"},
		{"localhost:5000/fixture:latest", ""},
		{"127.0.0.1:5000/fixture:latest", ""},
		{"[::1]:5000/fixture:latest", ""},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			calls := 0
			api := fakeSDK(t, func(r *http.Request) {
				auth, err := registry.DecodeAuthConfig(r.Header.Get("X-Registry-Auth"))
				if err != nil {
					t.Error("invalid auth encoding")
				}
				if tc.server == "https://index.docker.io/v1/" {
					if auth.Username != "dummy" || auth.Password != "dummy-secret" || auth.ServerAddress != tc.server {
						t.Error("Hub auth was not encoded correctly")
					}
				} else if r.Header.Get("X-Registry-Auth") != "" {
					t.Error("Hub credentials leaked to another registry")
				}
			})
			api.resolve = func(server string) (registry.AuthConfig, error) {
				calls++
				if server != tc.server {
					t.Error("credential lookup routed to wrong registry")
				}
				if server == "https://index.docker.io/v1/" {
					return registry.AuthConfig{Username: "dummy", Password: "dummy-secret", ServerAddress: server}, nil
				}
				return registry.AuthConfig{}, nil
			}
			pullDummy(t, api, tc.ref, image.PullOptions{})
			wantCalls := 1
			if tc.server == "" {
				wantCalls = 0
			}
			if calls != wantCalls {
				t.Error("unexpected credential resolver invocation count")
			}
		})
	}
}

func TestAuthenticatedPullPreservesExplicitOptions(t *testing.T) {
	api := fakeSDK(t, func(r *http.Request) {
		if r.Header.Get("X-Registry-Auth") != "explicit-auth" || r.URL.Query().Get("platform") != "linux/arm64" || r.URL.Query().Get("tag") != "latest" {
			t.Error("explicit pull options changed")
		}
	})
	api.resolve = func(string) (registry.AuthConfig, error) {
		t.Fatal("explicit authentication must bypass credential lookup")
		return registry.AuthConfig{}, nil
	}
	pullDummy(t, api, "busybox:latest", image.PullOptions{RegistryAuth: "explicit-auth", Platform: "linux/arm64"})
}

func TestAuthenticatedPullAnonymousAndPrivilegeFunc(t *testing.T) {
	api := fakeSDK(t, func(r *http.Request) {
		if r.Header.Get("X-Registry-Auth") != "" {
			t.Error("anonymous pull unexpectedly authenticated")
		}
	})
	api.resolve = func(string) (registry.AuthConfig, error) { return registry.AuthConfig{}, nil }
	pullDummy(t, api, "busybox:latest", image.PullOptions{})
	api.resolve = func(string) (registry.AuthConfig, error) {
		t.Fatal("explicit privilege function must bypass credential lookup")
		return registry.AuthConfig{}, nil
	}
	pullDummy(t, api, "busybox:latest", image.PullOptions{PrivilegeFunc: func(context.Context) (string, error) { return "explicit", nil }})
}

func TestAuthenticatedPullSanitizesHelperFailure(t *testing.T) {
	api := fakeSDK(t, func(*http.Request) { t.Error("pull should not run after helper failure") })
	api.resolve = func(string) (registry.AuthConfig, error) {
		return registry.AuthConfig{}, errors.New("dummy-secret helper stdout stderr")
	}
	_, err := api.ImagePull(context.Background(), "busybox:latest", image.PullOptions{})
	if err == nil || err.Error() != "integration Docker credential lookup failed" {
		t.Fatal("helper failure was not sanitized")
	}
}

func TestAuthenticatedPullIdentityToken(t *testing.T) {
	api := fakeSDK(t, func(r *http.Request) {
		auth, err := registry.DecodeAuthConfig(r.Header.Get("X-Registry-Auth"))
		if err != nil || auth.IdentityToken != "dummy-token" || auth.Password != "" {
			t.Error("identity token not forwarded correctly")
		}
	})
	api.resolve = func(server string) (registry.AuthConfig, error) {
		return registry.AuthConfig{IdentityToken: "dummy-token", ServerAddress: server}, nil
	}
	pullDummy(t, api, "busybox:latest", image.PullOptions{})
}

func fakeSDK(t *testing.T, check func(*http.Request)) *AuthenticatedClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{}\n")
	}))
	t.Cleanup(server.Close)
	api, err := client.NewClientWithOpts(client.WithHost(server.URL), client.WithVersion("1.47"))
	if err != nil {
		t.Fatal("create fake SDK client")
	}
	t.Cleanup(func() { _ = api.Close() })
	return &AuthenticatedClient{Client: api}
}

func pullDummy(t *testing.T, api *AuthenticatedClient, ref string, opts image.PullOptions) {
	t.Helper()
	stream, err := api.ImagePull(context.Background(), ref, opts)
	if err != nil {
		t.Fatal("dummy pull failed")
	}
	defer stream.Close()
	if _, err := io.Copy(io.Discard, stream); err != nil {
		t.Fatal("drain dummy pull")
	}
}

func TestSDKRequiresExplicitHost(t *testing.T) {
	if _, err := NewSDKClient(""); err == nil {
		t.Fatal("empty host must not select host daemon")
	}
	api := &AuthenticatedClient{}
	if _, err := api.ImagePull(context.Background(), "invalid reference!", image.PullOptions{}); err == nil || err.Error() != "invalid integration pull reference" {
		t.Fatal("invalid reference must fail safely before resolving credentials")
	}
}

// Compile-time evidence: the wrapper does not lose the log/inspect/Close API.
var _ interface {
	ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error)
	ContainerInspect(context.Context, string) (types.ContainerJSON, error)
	Close() error
} = (*AuthenticatedClient)(nil)
