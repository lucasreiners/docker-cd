package handler

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/gin-gonic/gin"
	"github.com/lucasreiners/docker-cd/internal/desiredstate"
)

type containerLogger interface {
	ContainerLogs(context.Context, string, container.LogsOptions) (io.ReadCloser, error)
}
type boundedLogWriter struct {
	writer    gin.ResponseWriter
	remaining int
}

func (w *boundedLogWriter) Write(data []byte) (int, error) {
	if w.remaining <= 0 {
		return 0, errors.New("log byte limit reached")
	}
	original := len(data)
	if len(data) > w.remaining {
		data = data[:w.remaining]
	}
	n, err := w.writer.Write(data)
	w.remaining -= n
	w.writer.Flush()
	if err == nil && n < original {
		err = errors.New("log byte limit reached")
	}
	return n, err
}
func containerLogsHandler(api DockerAPI, lister ContainerLister, store *desiredstate.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		path, id := c.Query("stack"), c.Query("container")
		known := false
		for _, stack := range store.GetStacks() {
			if stack.Path == path {
				known = true
				break
			}
		}
		if !known || id == "" {
			c.JSON(404, gin.H{"error": "Managed stack or container not found"})
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Minute)
		defer cancel()
		containers, err := lister.GetContainers(ctx, path)
		if err != nil {
			c.JSON(503, gin.H{"error": err.Error()})
			return
		}
		allowed := false
		for _, info := range containers {
			if info.ID == id {
				allowed = true
				break
			}
		}
		if !allowed {
			c.JSON(404, gin.H{"error": "Container does not belong to this stack"})
			return
		}
		tail, _ := strconv.Atoi(c.DefaultQuery("tail", "200"))
		if tail < 1 || tail > 1000 {
			tail = 200
		}
		inspected, err := api.ContainerInspect(ctx, id)
		if err != nil {
			c.JSON(503, gin.H{"error": err.Error()})
			return
		}
		stream, err := api.(containerLogger).ContainerLogs(ctx, id, container.LogsOptions{ShowStdout: true, ShowStderr: true, Timestamps: true, Tail: strconv.Itoa(tail), Follow: c.Query("follow") == "true"})
		if err != nil {
			c.JSON(503, gin.H{"error": err.Error()})
			return
		}
		defer stream.Close()
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("Cache-Control", "no-store")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)
		c.Writer.Flush()
		writer := &boundedLogWriter{writer: c.Writer, remaining: 1 << 20}
		if inspected.Config != nil && inspected.Config.Tty {
			_, _ = io.Copy(writer, stream)
		} else {
			_, _ = stdcopy.StdCopy(writer, writer, stream)
		}
	}
}
