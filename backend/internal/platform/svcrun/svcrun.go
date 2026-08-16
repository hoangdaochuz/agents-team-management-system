// Package svcrun is the shared service runtime: structured logging, signal-driven
// graceful shutdown, and a health endpoint. Each service's cmd/main.go calls Run
// with a function that registers its routes on a *http.ServeMux.
package svcrun

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Registrar registers routes on mux (under /api or root). The service owns its
// handler tree; svcrun only adds /healthz. ctx is the service lifecycle context
// (cancelled on SIGINT/SIGTERM) so services can thread it into Kafka consumer
// groups for graceful drain.
type Registrar func(ctx context.Context, mux *http.ServeMux, log *slog.Logger) error

// Run boots a named service: JSON logging, route registration, /healthz, a
// server with sane timeouts, and graceful shutdown on SIGINT/SIGTERM.
func Run(name, addr string, register Registrar) {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("svc", name)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "svc": name})
	})

	if err := register(ctx, mux, log); err != nil {
		log.Error("route registration failed", "error", err)
		os.Exit(1)
	}

	// request-id + recovery middleware wrap the mux.
	handler := recoverMiddleware(requestIDMiddleware(mux))

	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       0, // SSE/long requests: no read timeout
		WriteTimeout:      0, // SSE: no write timeout
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("http server starting", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server error", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutdown signal received", "svc", name)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	}
	log.Info("server stopped", "svc", name)
}
