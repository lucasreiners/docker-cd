//go:build integration

package harness

import (
	"context"
	"encoding/json"
	"io"
	"sync"

	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

// SlowPull injects Docker Engine NDJSON at the SDK boundary, not UI/SSE events.
// The underlying image is really pulled in DinD after the controlled stream.
// Layer sizes are simulated: CI does not download hundreds of MiB to test timing.
type SlowPull struct {
	docker.DockerAPI
	mu     sync.Mutex
	armed  bool
	fail   bool
	steps  chan struct{}
	active bool
}

func (p *SlowPull) Arm(fail bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.armed, p.fail = true, fail
	p.steps = make(chan struct{}, 4)
}

func (p *SlowPull) Advance() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.steps != nil {
		select {
		case p.steps <- struct{}{}:
		default:
		}
	}
}

func (p *SlowPull) Active() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.active }

func (p *SlowPull) ImagePull(ctx context.Context, ref string, opts image.PullOptions) (io.ReadCloser, error) {
	p.mu.Lock()
	if !p.armed {
		p.mu.Unlock()
		return p.DockerAPI.ImagePull(ctx, ref, opts)
	}
	p.armed = false
	p.active = true
	steps, fail := p.steps, p.fail
	p.mu.Unlock()
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		defer func() { p.mu.Lock(); p.active = false; p.mu.Unlock() }()
		encoder := json.NewEncoder(writer)
		emit := func(id, status string, current, total int64) bool {
			return encoder.Encode(map[string]interface{}{"id": id, "status": status,
				"progressDetail": map[string]int64{"current": current, "total": total}}) == nil
		}
		wait := func() bool {
			select {
			case <-steps:
				return true
			case <-ctx.Done():
				return false
			}
		}
		const mib = 1024 * 1024
		if !emit("layer-a", "Downloading", mib, 128*mib) || !emit("layer-b", "Downloading", 2*mib, 256*mib) || !wait() {
			return
		}
		if !emit("layer-a", "Downloading", 32*mib, 128*mib) || !emit("layer-b", "Downloading", 64*mib, 256*mib) || !wait() {
			return
		}
		if fail {
			_ = encoder.Encode(map[string]string{"error": "fixture: registry connection interrupted"})
			return
		}
		if !emit("layer-a", "Download complete", 128*mib, 128*mib) || !emit("layer-a", "Extracting", 16*mib, 128*mib) || !emit("layer-b", "Downloading", 96*mib, 256*mib) || !wait() {
			return
		}
		if !emit("layer-a", "Pull complete", 128*mib, 128*mib) || !emit("layer-b", "Extracting", 32*mib, 256*mib) || !wait() {
			return
		}
		if !emit("layer-b", "Pull complete", 256*mib, 256*mib) {
			return
		}
		actual, err := p.DockerAPI.ImagePull(ctx, ref, opts)
		if err != nil {
			writer.CloseWithError(err)
			return
		}
		defer actual.Close()
		_, err = io.Copy(writer, actual)
		if err != nil {
			writer.CloseWithError(err)
		}
	}()
	// Ensure a paused pipe unblocks promptly on cancellation or early read errors.
	return &pullReadCloser{PipeReader: reader, stop: context.AfterFunc(ctx, func() { reader.CloseWithError(ctx.Err()) })}, nil
}

type pullReadCloser struct {
	*io.PipeReader
	stop func() bool
}

func (r *pullReadCloser) Close() error { r.stop(); return r.PipeReader.Close() }
