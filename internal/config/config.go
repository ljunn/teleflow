package config

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddress    string
	DataDirectory    string
	DatabasePath     string
	GitHubRepository string
	PublicURL        string
	TelegramAPIID    int
	TelegramAPIHash  string
	RelayBotToken    string
	SessionKey       []byte
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

	apiID := 0
	if raw := strings.TrimSpace(os.Getenv("TELEFLOW_TELEGRAM_API_ID")); raw != "" {
		if parsed, parseErr := strconv.Atoi(raw); parseErr == nil && parsed > 0 {
			apiID = parsed
		}
	}
	sessionKey, err := loadOrCreateSessionKey(absDataDir)
	if err != nil {
		return Config{}, err
	}

	return Config{
		ListenAddress:    env("TELEFLOW_ADDR", ":8080"),
		DataDirectory:    absDataDir,
		DatabasePath:     filepath.Join(absDataDir, "teleflow.db"),
		GitHubRepository: strings.TrimSpace(os.Getenv("TELEFLOW_GITHUB_REPOSITORY")),
		PublicURL:        strings.TrimRight(env("TELEFLOW_PUBLIC_URL", "http://localhost:8080"), "/"),
		TelegramAPIID:    apiID,
		TelegramAPIHash:  strings.TrimSpace(os.Getenv("TELEFLOW_TELEGRAM_API_HASH")),
		RelayBotToken:    strings.TrimSpace(os.Getenv("TELEFLOW_RELAY_BOT_TOKEN")),
		SessionKey:       sessionKey,
	}, nil
}

func loadOrCreateSessionKey(dataDir string) ([]byte, error) {
	path := filepath.Join(dataDir, "session.key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, fmt.Errorf("session encryption key must be 32 bytes")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read session encryption key: %w", err)
	}

	key = make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate session encryption key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return loadOrCreateSessionKey(dataDir)
	}
	if err != nil {
		return nil, fmt.Errorf("create session encryption key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		file.Close()
		return nil, fmt.Errorf("write session encryption key: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return nil, fmt.Errorf("sync session encryption key: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("close session encryption key: %w", err)
	}
	return key, nil
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
