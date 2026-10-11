package handler

import (
	"bytes"
	"context"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

type logAPI struct {
	DockerAPI
	tty     bool
	calls   int
	options container.LogsOptions
}

func (a *logAPI) ContainerInspect(context.Context, string) (types.ContainerJSON, error) {
	return types.ContainerJSON{Config: &container.Config{Tty: a.tty}}, nil
}
func (a *logAPI) ContainerLogs(_ context.Context, _ string, options container.LogsOptions) (io.ReadCloser, error) {
	a.calls++
	a.options = options
	if a.tty {
		return io.NopCloser(strings.NewReader("tty output\n")), nil
	}
	var output bytes.Buffer
	_, _ = stdcopy.NewStdWriter(&output, stdcopy.Stdout).Write([]byte("standard output\n"))
	_, _ = stdcopy.NewStdWriter(&output, stdcopy.Stderr).Write([]byte("standard error\n"))
	return io.NopCloser(&output), nil
}

type logLister struct{}

func (logLister) GetContainers(context.Context, string) ([]desiredstate.ContainerInfo, error) {
	return []desiredstate.ContainerInfo{{ID: "managed", Name: "web"}}, nil
}

func TestContainerLogsDemultiplexAndEnforceStackMembership(t *testing.T) {
	for _, tty := range []bool{false, true} {
		api := &logAPI{tty: tty}
		store := desiredstate.NewStore()
		store.Set(&desiredstate.Snapshot{Stacks: []desiredstate.StackRecord{{Path: "apps/web"}}})
		r := gin.New()
		r.GET("/logs", containerLogsHandler(api, logLister{}, store))
		invalid := httptest.NewRecorder()
		r.ServeHTTP(invalid, httptest.NewRequest("GET", "/logs?stack=apps/web&container=other", nil))
		if invalid.Code != 404 || api.calls != 0 {
			t.Fatal("unmanaged container logs requested")
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest("GET", "/logs?stack=apps/web&container=managed&follow=true&tail=99999", nil))
		if rec.Code != 200 {
			t.Fatal(rec.Body.String())
		}
		want := "standard output\nstandard error\n"
		if tty {
			want = "tty output\n"
		}
		if rec.Body.String() != want {
			t.Fatalf("bad stream %q", rec.Body.String())
		}
		if api.options.Tail != "200" || !api.options.Follow {
			t.Fatalf("wrong bounded options: %+v", api.options)
		}
	}
}
