package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	documentsPath, redactionsPath := datasetPaths()
	logger.Info("startup_loading_dataset",
		slog.String("documents_path", documentsPath),
		slog.String("redactions_path", redactionsPath),
	)

	documents, redactions, err := LoadDataset(documentsPath, redactionsPath)
	if err != nil {
		logger.Error("startup_validation_failed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	store := NewStore(documents, redactions)
	summary := store.Summary()
	logger.Info("startup_dataset_loaded",
		slog.Int("document_count", summary.TotalDocuments),
		slog.Int("redaction_count", summary.TotalRedactions),
		slog.Int("ready_count", summary.Ready),
		slog.Int("needs_review_count", summary.NeedsReview),
		slog.Int("failed_count", summary.Failed),
	)

	router := NewRouter(logger, store)
	server := &http.Server{
		Addr:    ":8080",
		Handler: router,
	}

	logger.Info("server_listening", slog.String("addr", server.Addr))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server_stopped", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
