package main

import (
	"context"
	"errors"
	"github.com/somakalla1-droid/gke-request-info-service/internal/observability"
	"github.com/somakalla1-droid/gke-request-info-service/internal/server"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	port := env("PORT", "8080")
	config := server.Config{
		AppName:            "gke-request-info-service",
		Version:            env("APP_VERSION", "dev"),
		ProjectID:          env("GOOGLE_CLOUD_PROJECT", ""),
		ClusterName:        env("CLUSTER_NAME", "local"),
		Region:             env("REGION", "local"),
		PodName:            env("POD_NAME", hostname()),
		ResponseServiceURL: env("RESPONSE_SERVICE_URL", ""),
	}

	shutdownTracing := func(context.Context) error { return nil }
	if env("OBSERVABILITY_ENABLED", "false") == "true" {
		telemetryConfig := observability.Config{
			ProjectID:      config.ProjectID,
			ServiceName:    config.AppName,
			ServiceVersion: config.Version,
			ClusterName:    config.ClusterName,
			Region:         config.Region,
		}
		var err error
		shutdownTracing, err = observability.StartTracing(context.Background(), telemetryConfig)
		if err != nil {
			log.Printf(`{"severity":"ERROR","message":"tracing initialization failed","error":%q}`, err.Error())
			shutdownTracing = func(context.Context) error { return nil }
		}
		if err := observability.StartProfiler(telemetryConfig); err != nil {
			log.Printf(`{"severity":"ERROR","message":"profiler initialization failed","error":%q}`, err.Error())
		}
	}

	h := server.New(config)
	httpServer := &http.Server{Addr: ":" + port, Handler: h, ReadHeaderTimeout: 5 * time.Second}
	stopContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	serverError := make(chan error, 1)

	log.Printf(`{"severity":"INFO","message":"server starting","port":%q}`, port)
	go func() { serverError <- httpServer.ListenAndServe() }()

	select {
	case <-stopContext.Done():
		log.Printf(`{"severity":"INFO","message":"server shutdown requested"}`)
	case err := <-serverError:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Printf(`{"severity":"ERROR","message":"server failed","error":%q}`, err.Error())
		}
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownContext); err != nil {
		log.Printf(`{"severity":"ERROR","message":"HTTP shutdown failed","error":%q}`, err.Error())
	}
	if err := shutdownTracing(shutdownContext); err != nil {
		log.Printf(`{"severity":"ERROR","message":"trace flush failed","error":%q}`, err.Error())
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}
