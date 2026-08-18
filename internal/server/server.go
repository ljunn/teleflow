package server

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/updater"
	"github.com/ljunn/teleflow/internal/version"
	webassets "github.com/ljunn/teleflow/internal/web"
)

func New(cfg config.Config, db *sql.DB, updateService *updater.Service, logger *slog.Logger) http.Handler {
	return NewWithRestart(cfg, db, updateService, logger, func() {})
}

func NewWithRestart(cfg config.Config, db *sql.DB, updateService *updater.Service, logger *slog.Logger, restart func()) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), requestLogger(logger))

	router.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.GET("/health/ready", func(c *gin.Context) {
		if err := db.PingContext(c.Request.Context()); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	api := router.Group("/api/v1")
	auth := newAuthService(db)
	api.GET("/auth/status", auth.status)
	api.POST("/auth/setup", auth.setup)
	api.POST("/auth/login", auth.login)
	api.POST("/auth/logout", auth.logout)

	protected := api.Group("")
	protected.Use(auth.middleware())
	protected.GET("/system/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":      "Teleflow",
			"version":   version.Version,
			"commit":    version.Commit,
			"buildDate": version.BuildDate,
			"publicUrl": cfg.PublicURL,
		})
	})
	protected.GET("/system/update/check", func(c *gin.Context) {
		release, err := updateService.Check(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, release)
	})
	protected.POST("/system/update", func(c *gin.Context) {
		result, err := updateService.Update(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if !result.Updated {
			c.JSON(http.StatusConflict, gin.H{"error": "当前没有可用的新版本", "release": result.Release})
			return
		}
		c.JSON(http.StatusOK, result)
		go func() {
			time.Sleep(750 * time.Millisecond)
			restart()
		}()
	})
	protected.GET("/overview", func(c *gin.Context) {
		var accounts, online, pendingJobs, relayedToday int
		row := db.QueryRowContext(c.Request.Context(), `
			SELECT
				(SELECT COUNT(*) FROM telegram_accounts),
				(SELECT COUNT(*) FROM telegram_accounts WHERE status = 'online'),
				(SELECT COUNT(*) FROM jobs WHERE status = 'pending') +
				(SELECT COUNT(*) FROM discovery_tasks WHERE status IN ('pending_connection', 'ready', 'running')) +
				(SELECT COUNT(*) FROM campaigns WHERE status IN ('pending_connection', 'ready', 'running')),
				(SELECT COUNT(*) FROM relay_links WHERE created_at >= datetime('now', 'start of day'))
		`)
		if err := row.Scan(&accounts, &online, &pendingJobs, &relayedToday); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "read overview"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"accounts":     accounts,
			"online":       online,
			"pendingJobs":  pendingJobs,
			"relayedToday": relayedToday,
		})
	})
	registerOperationsRoutes(protected, cfg, db)

	webassets.Register(router)
	return router
}

func requestLogger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		c.Next()
		logger.Info("http request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
	}
}
