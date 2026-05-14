package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ruipereira-pt/yuno-ai-challenge/internal/api"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/config"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/service"
	"github.com/ruipereira-pt/yuno-ai-challenge/internal/store"
)

func main() {
	cfg := config.Load()

	windowStore := store.NewWindowStore()
	scorer := service.NewScorer()
	alerts := service.NewAlertEvaluator(cfg.Alert, scorer)
	healthSvc := service.NewHealthService(windowStore, scorer, alerts, cfg.MaxEventAge, cfg.MaxFutureSkew)

	handler := api.NewHandler(healthSvc).WithStreamConfig(api.StreamConfig{
		Enabled:        cfg.WS.Enabled,
		MaxFrameBytes:  cfg.WS.MaxFrameBytes,
		ReadTimeout:    cfg.WS.ReadTimeout,
		BatchSize:      cfg.WS.BatchSize,
		FlushInterval:  cfg.WS.FlushInterval,
		QueueSize:      cfg.WS.QueueSize,
		AllowedOrigins: cfg.WS.AllowedOrigins,
		StreamToken:    cfg.WS.StreamToken,
	})
	router := api.NewRouter(handler)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB
	}

	log.Printf("starting psp health monitoring service on %s", cfg.HTTPAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Printf("server failed: %v", err)
		os.Exit(1)
	}
}
