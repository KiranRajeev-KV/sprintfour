package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadDotEnv loads the closest .env file into the current process without overwriting existing variables.
func LoadDotEnv() (string, error) {
	path, ok, err := FindDotEnv()
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}

	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open .env: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return "", fmt.Errorf(".env line %d missing '='", lineNumber)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return "", fmt.Errorf(".env line %d has empty key", lineNumber)
		}

		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			return "", fmt.Errorf("set env %s: %w", key, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("read .env: %w", err)
	}

	return path, nil
}

// FindDotEnv searches the working directory and parent directories for a .env file.
func FindDotEnv() (string, bool, error) {
	workingDir, err := os.Getwd()
	if err != nil {
		return "", false, fmt.Errorf("get working directory: %w", err)
	}

	initialCandidates := []string{
		filepath.Join(workingDir, ".env"),
		filepath.Join(workingDir, "backend", ".env"),
	}
	for _, candidate := range initialCandidates {
		info, statErr := os.Stat(candidate)
		if statErr == nil && !info.IsDir() {
			return candidate, true, nil
		}
		if statErr != nil && !os.IsNotExist(statErr) {
			return "", false, fmt.Errorf("stat .env: %w", statErr)
		}
	}

	for dir := workingDir; ; dir = filepath.Dir(dir) {
		candidates := []string{
			filepath.Join(dir, ".env"),
			filepath.Join(dir, "backend", ".env"),
		}
		for _, candidate := range candidates {
			info, statErr := os.Stat(candidate)
			if statErr == nil && !info.IsDir() {
				return candidate, true, nil
			}
			if statErr != nil && !os.IsNotExist(statErr) {
				return "", false, fmt.Errorf("stat .env: %w", statErr)
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}

	return "", false, nil
}
