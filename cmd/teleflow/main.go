package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/database"
	"github.com/ljunn/teleflow/internal/server"
	"github.com/ljunn/teleflow/internal/updater"
	"github.com/ljunn/teleflow/internal/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "version") {
		fmt.Printf("teleflow %s (commit %s, built %s)\n", version.Version, version.Commit, version.BuildDate)
		return
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	updateService := updater.New(updater.Options{
		Repository: cfg.GitHubRepository,
		Current:    version.Version,
	})

	httpServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           server.NewWithRestart(cfg, db, updateService, logger, func() { os.Exit(0) }),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	go func() {
		logger.Info("teleflow started",
			"address", cfg.ListenAddress,
			"version", version.Version,
			"commit", version.Commit,
		)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown", "error", err)
	}
}
