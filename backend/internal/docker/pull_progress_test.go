package docker_test

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/image"
	"github.com/lucasreiners/docker-cd/internal/docker"
)

func TestPullProgress_LayersAndPhaseChangesAreNotThrottledAway(t *testing.T) {
	stream := `{"id":"a","status":"Downloading","progressDetail":{"current":10,"total":100}}
{"id":"b","status":"Downloading","progressDetail":{"current":20,"total":200}}
{"id":"a","status":"Downloading","progressDetail":{"current":11,"total":100}}
{"id":"a","status":"Download complete"}
{"id":"a","status":"Extracting","progressDetail":{"current":1,"total":100}}
{"id":"a","status":"Pull complete"}
{"id":"b","status":"Pull complete"}
`
	api := &mockDockerAPI{imagePullFn: func(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(stream)), nil
	}}
	var events []docker.PullProgress
	if err := docker.NewClient(api).PullImages(context.Background(), []string{"large"}, func(p docker.PullProgress) { events = append(events, p) }); err != nil {
		t.Fatal(err)
	}
	if len(events) != 7 {
		t.Fatalf("expected two layer starts, four transitions and image completion: %+v", events)
	}
	if events[1].LayerID != "b" || events[1].BytesCurrent != 20 || events[1].BytesTotal != 200 {
		t.Fatalf("lost layer progress: %+v", events)
	}
	for _, p := range events[:len(events)-1] {
		if p.Done {
			t.Fatalf("layer completed whole image: %+v", p)
		}
	}
	if !events[len(events)-1].Done {
		t.Fatal("missing image completion")
	}
}

func TestPullProgress_StreamFailureNeverCompletesImage(t *testing.T) {
	api := &mockDockerAPI{imagePullFn: func(context.Context, string, image.PullOptions) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("{\"id\":\"a\",\"status\":\"Downloading\"}\n{\"error\":\"connection interrupted\"}\n")), nil
	}}
	err := docker.NewClient(api).PullImages(context.Background(), []string{"large"}, func(p docker.PullProgress) {
		if p.Done {
			t.Fatal("failed stream emitted completion")
		}
	})
	if err == nil || !strings.Contains(err.Error(), "connection interrupted") {
		t.Fatalf("missing pull error: %v", err)
	}
}
