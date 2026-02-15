package main

import (
	"fmt"
	"log"

	"github.com/lucasreiners/docker-cd/internal/config"
	"github.com/lucasreiners/docker-cd/internal/docker"
	handler "github.com/lucasreiners/docker-cd/internal/http"
)

func main() {
	cfg := config.Load()
	runner := &docker.ExecRunner{}

	router := handler.NewRouter(runner, cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	log.Printf("Docker-CD starting on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
