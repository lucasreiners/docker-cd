//go:build integration

package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lucasreiners/docker-cd/tests/integration/harness"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	h, err := harness.New(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close()
	h.EnableControls()
	server := &http.Server{Addr: "127.0.0.1:18080", Handler: h.Router, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Print(err)
	}
}
