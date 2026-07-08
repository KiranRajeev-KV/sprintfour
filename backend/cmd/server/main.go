package main

import (
	"backend/internal/config"
	"backend/internal/detector"
	"backend/internal/httpapi"
	storepkg "backend/internal/store"
	"backend/internal/worker"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	runtimeConfig, err := config.LoadRuntime()
	if err != nil {
		logger.Error("config_load_failed", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if runtimeConfig.DotEnvPath != "" {
		logger.Info("dotenv_loaded", slog.String("path", runtimeConfig.DotEnvPath))
	} else {
		logger.Info("dotenv_not_found")
	}

	store := storepkg.NewStore(nil, nil)
	summary := store.Summary()
	logger.Info("startup_store_ready",
		slog.Int("document_count", summary.TotalDocuments),
		slog.Int("redaction_count", summary.TotalRedactions),
		slog.Int("ready_count", summary.Ready),
		slog.Int("needs_review_count", summary.NeedsReview),
		slog.Int("failed_count", summary.Failed),
	)

	detector := detector.NewRuntimeDetector(logger, runtimeConfig.Detector)
	workerPool := worker.NewWorkerPool(logger, store, detector, runtimeConfig.WorkerCount, runtimeConfig.QueueDepth)
	store.SetJobSubmitter(workerPool)
	workerPool.Start()

	router := httpapi.NewRouter(logger, store)
	server := &http.Server{
		Addr:    runtimeConfig.HTTPAddr,
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
