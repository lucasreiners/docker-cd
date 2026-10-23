//go:build integration

package dind

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/docker/docker/api/types/image"
)

// TestAuthenticatedSDKPreflight is deliberately small and opt-in via the
// integration tag. It never prints pull payloads, credentials or helper errors.
func TestAuthenticatedSDKPreflight(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	auth, err := cliCredentials("https://index.docker.io/v1/")
	if err != nil {
		t.Fatal("Docker Hub credential lookup failed (details suppressed)")
	}
	if auth.Username == "" && auth.Password == "" && auth.IdentityToken == "" {
		t.Skip("authenticated SDK preflight requires an existing Docker Hub CLI login")
	}
	env := StartT(t)
	api, err := NewSDKClient(env.DockerHost)
	if err != nil {
		t.Fatal("create private SDK client failed (details suppressed)")
	}
	defer api.Close()
	stream, err := api.ImagePull(ctx, "busybox:1.38.0", image.PullOptions{})
	if err != nil {
		t.Fatal("authenticated SDK pull failed (details suppressed)")
	}
	defer stream.Close()
	decoder := json.NewDecoder(stream)
	for {
		var event struct {
			Error string `json:"error"`
		}
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil || event.Error != "" {
			t.Fatal("authenticated SDK pull stream failed (details suppressed)")
		}
	}
	if _, _, err := api.ImageInspectWithRaw(ctx, "busybox:1.38.0"); err != nil {
		t.Fatal("private SDK image inspection failed (details suppressed)")
	}
	t.Log("authenticated Docker Hub SDK pull and private DinD image inspection succeeded")
}
