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
	api.GET("/system/info", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":      "Teleflow",
			"version":   version.Version,
			"commit":    version.Commit,
			"buildDate": version.BuildDate,
			"publicUrl": cfg.PublicURL,
		})
	})
	api.GET("/system/update/check", func(c *gin.Context) {
		release, err := updateService.Check(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, release)
	})
	api.GET("/overview", func(c *gin.Context) {
		var accounts, online, pendingJobs, relayedToday int
		row := db.QueryRowContext(c.Request.Context(), `
			SELECT
				(SELECT COUNT(*) FROM telegram_accounts),
				(SELECT COUNT(*) FROM telegram_accounts WHERE status = 'online'),
				(SELECT COUNT(*) FROM jobs WHERE status = 'pending'),
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
