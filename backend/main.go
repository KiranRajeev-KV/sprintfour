package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	dotEnvPath, err := loadDotEnv()
	if err != nil {
		logger.Error("dotenv_load_failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if dotEnvPath != "" {
		logger.Info("dotenv_loaded", slog.String("path", dotEnvPath))
	} else {
		logger.Info("dotenv_not_found")
	}

	store := NewStore(nil, nil)
	summary := store.Summary()
	logger.Info("startup_store_ready",
		slog.Int("document_count", summary.TotalDocuments),
		slog.Int("redaction_count", summary.TotalRedactions),
		slog.Int("ready_count", summary.Ready),
		slog.Int("needs_review_count", summary.NeedsReview),
		slog.Int("failed_count", summary.Failed),
	)

	workerCount := envInt("WORKER_COUNT", 8)
	queueDepth := envInt("QUEUE_DEPTH", 200)
	workerPool = NewWorkerPool(store, workerCount, queueDepth)
	workerPool.Start()

	router := NewRouter(logger, store)
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	go func() {
		logger.Info("server_listening", slog.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_stopped", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Info("shutdown_requested", slog.String("signal", sig.String()))

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("http_shutdown_error", slog.String("error", err.Error()))
	}

	if err := workerPool.Shutdown(shutdownCtx); err != nil {
		logger.Error("worker_pool_shutdown_error", slog.String("error", err.Error()))
	}

	logger.Info("shutdown_complete")
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return val
}
