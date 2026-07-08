package config

import (
	"backend/internal/detector"
	"os"
	"strconv"
)

// Runtime holds the server's process configuration loaded from the environment.
type Runtime struct {
	DotEnvPath  string
	HTTPAddr    string
	WorkerCount int
	QueueDepth  int
	Detector    detector.Config
}

// LoadRuntime loads dotenv values, if present, and returns the typed runtime configuration.
func LoadRuntime() (Runtime, error) {
	dotEnvPath, err := LoadDotEnv()
	if err != nil {
		return Runtime{}, err
	}

	return Runtime{
		DotEnvPath:  dotEnvPath,
		HTTPAddr:    envString("HTTP_ADDR", ":8080"),
		WorkerCount: envInt("WORKER_COUNT", 8),
		QueueDepth:  envInt("QUEUE_DEPTH", 200),
		Detector: detector.Config{
			GLiNEREnabled:        envBool("GLINER_ENABLED", false),
			GLiNERURL:            envString("GLINER_URL", "http://127.0.0.1:8090"),
			GLiNERTimeoutMS:      envInt("GLINER_TIMEOUT_MS", 2500),
			GLiNERMaxConcurrency: envInt("GLINER_MAX_CONCURRENCY", 1),
		},
	}, nil
}

func envInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}

	switch raw {
	case "1", "true", "TRUE", "True", "yes", "YES", "on", "ON":
		return true
	case "0", "false", "FALSE", "False", "no", "NO", "off", "OFF":
		return false
	default:
		return fallback
	}
}

func envString(key, fallback string) string {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	return raw
}
