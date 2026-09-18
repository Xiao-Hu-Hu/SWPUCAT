package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"SWPUCAT/internal/agent"
	"SWPUCAT/internal/infrastructure/config"
	agenthttp "SWPUCAT/internal/interfaces/agenthttp"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}
	if err := cfg.Agent.Validate(); err != nil {
		log.Fatalf("invalid agent configuration: %v", err)
	}
	engine, err := agent.NewEngine(context.Background(), cfg.Agent)
	if err != nil {
		log.Fatalf("failed to initialize Eino agent: %v", err)
	}

	gin.SetMode(cfg.Server.Mode)
	router := gin.New()
	router.Use(gin.Recovery())
	handler := agenthttp.NewHandler(engine, cfg.Agent.InternalSecret)
	router.GET("/healthz", handler.Health)
	router.POST("/v1/chat", handler.Chat)
	router.POST("/v1/chat/stream", handler.ChatStream)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Agent.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: cfg.Agent.EffectiveExecutionTimeout() + 10*time.Second,
		IdleTimeout:  60 * time.Second,
	}
	log.Printf("Agent service listening on %s (enabled=%t)", server.Addr, engine.Enabled())
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("agent server failed: %v", err)
	}
}
