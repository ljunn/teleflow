package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ListenAddress    string
	DataDirectory    string
	DatabasePath     string
	GitHubRepository string
	PublicURL        string
}

func Load() (Config, error) {
	dataDir := env("TELEFLOW_DATA_DIR", "./data")
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return Config{}, fmt.Errorf("resolve data directory: %w", err)
	}
	if err := os.MkdirAll(absDataDir, 0o750); err != nil {
		return Config{}, fmt.Errorf("create data directory: %w", err)
	}

	return Config{
		ListenAddress:    env("TELEFLOW_ADDR", ":8080"),
		DataDirectory:    absDataDir,
		DatabasePath:     filepath.Join(absDataDir, "teleflow.db"),
		GitHubRepository: strings.TrimSpace(os.Getenv("TELEFLOW_GITHUB_REPOSITORY")),
		PublicURL:        strings.TrimRight(env("TELEFLOW_PUBLIC_URL", "http://localhost:8080"), "/"),
	}, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
