package dind

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// PublishedImage records the daemon's image ID and the registry manifest digest.
// They are independently observed, even when a containerd image store reports
// the same value for both; no configuration-digest equivalence is assumed.
type PublishedImage struct {
	ImageID string
	Digest  string
}

// MutableImage owns a mutable tag in a registry running inside a private DinD.
// Use one fixture per Env. The registry stays running across updater pruning;
// Env.Cleanup owns its lifetime. No command targets the host Docker daemon.
type MutableImage struct {
	Ref     string
	Initial PublishedImage
	host    string
}

// NewMutableImage publishes revision A using a stopped BusyBox seed container.
// localhost is resolved by the DinD daemon, not the machine running this test.
func NewMutableImage(ctx context.Context, env *Env) (*MutableImage, error) {
	if env == nil || env.DockerHost == "" {
		return nil, fmt.Errorf("mutable image requires an explicit private Docker host")
	}
	f := &MutableImage{Ref: "localhost:5000/docker-cd-fixture:current", host: env.DockerHost}
	if _, err := f.command(ctx, "run", "-d", "--name", "docker-cd-fixture-registry", "-p", "127.0.0.1:5000:5000", "registry:2"); err != nil {
		return nil, err
	}
	if _, err := f.command(ctx, "pull", "busybox:1.38.0"); err != nil {
		return nil, err
	}
	// Probe through the inner network namespace, never the host's loopback.
	readyCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		if _, err := f.command(readyCtx, "run", "--rm", "--network", "host", "busybox:1.38.0", "wget", "-qO-", "http://127.0.0.1:5000/v2/"); err == nil {
			break
		}
		select {
		case <-readyCtx.Done():
			return nil, fmt.Errorf("wait for private registry: %w", readyCtx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	initial, err := f.publish(ctx, "busybox:1.38.0", "A")
	if err != nil {
		return nil, err
	}
	f.Initial = initial
	return f, nil
}

// PublishChanged publishes B, then restores the LOCAL tag to A without pushing
// A. Call only after the unchanged updater cycle: its prune can remove unused B.
// The next updater must therefore pull remote B over local/running A.
func (f *MutableImage) PublishChanged(ctx context.Context) (PublishedImage, error) {
	changed, err := f.publish(ctx, f.Initial.ImageID, "B")
	if err != nil {
		return PublishedImage{}, err
	}
	if changed.ImageID == f.Initial.ImageID || changed.Digest == f.Initial.Digest {
		return PublishedImage{}, fmt.Errorf("changed revision must have distinct image ID and manifest digest")
	}
	if _, err := f.command(ctx, "tag", f.Initial.ImageID, f.Ref); err != nil {
		return PublishedImage{}, err
	}
	return changed, nil
}

func (f *MutableImage) publish(ctx context.Context, base, revision string) (PublishedImage, error) {
	seed, err := f.command(ctx, "create", base, "sleep", "86400")
	if err != nil {
		return PublishedImage{}, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = f.command(cleanupCtx, "rm", seed)
	}()
	if _, err := f.command(ctx, "commit", "--change", "LABEL docker-cd.fixture.revision="+revision, seed, f.Ref); err != nil {
		return PublishedImage{}, err
	}
	// CombinedOutput drains the complete push stream, including registry errors.
	output, err := f.command(ctx, "push", f.Ref)
	if err != nil {
		return PublishedImage{}, err
	}
	match := regexp.MustCompile(`digest: (sha256:[a-f0-9]{64})`).FindStringSubmatch(output)
	if len(match) != 2 {
		return PublishedImage{}, fmt.Errorf("push did not report a manifest digest: %s", output)
	}
	imageID, err := f.LocalImageID(ctx)
	if err != nil {
		return PublishedImage{}, err
	}
	return PublishedImage{ImageID: imageID, Digest: match[1]}, nil
}

// LocalImageID inspects the current tag without pulling it.
func (f *MutableImage) LocalImageID(ctx context.Context) (string, error) {
	return f.command(ctx, "image", "inspect", "--format", "{{.Id}}", f.Ref)
}

// RemoteDigest reads the registry manifest from inside DinD without changing
// the local tag. The probe image is the retained A image, so pruning BusyBox's
// original tag does not introduce another external pull.
func (f *MutableImage) RemoteDigest(ctx context.Context) (string, error) {
	manifest, err := f.command(ctx, "run", "--rm", "--network", "host", f.Initial.ImageID,
		"wget", "-qO-", "--header=Accept: application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json",
		"http://127.0.0.1:5000/v2/docker-cd-fixture/manifests/current")
	if err != nil {
		return "", err
	}
	// command trims only output whitespace; JSON has no trailing whitespace in
	// registry responses. Validate before hashing to reject non-manifest bodies.
	var body struct {
		SchemaVersion int `json:"schemaVersion"`
	}
	if err := json.Unmarshal([]byte(manifest), &body); err != nil || body.SchemaVersion != 2 {
		return "", fmt.Errorf("invalid registry manifest: %q", manifest)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(manifest))), nil
}

func (f *MutableImage) command(ctx context.Context, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, "docker", append([]string{"-H", f.host}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("private Docker %v: %w: %s", args, err, output)
	}
	return strings.TrimSpace(string(output)), nil
}
