package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
)

const (
	telegramAPIIDSetting   = "telegram_api_id"
	telegramAPIHashSetting = "telegram_api_hash_ciphertext"
	telegramAPIHashPurpose = "telegram-api-hash"
)

var telegramAPIHashPattern = regexp.MustCompile(`^[A-Fa-f0-9]{32}$`)

type telegramCredentials struct {
	APIID   int
	APIHash string
	Source  string
}

func registerTelegramSettingsRoutes(group *gin.RouterGroup, cfg config.Config, db *sql.DB) {
	group.GET("/telegram/settings", func(c *gin.Context) {
		credentials := loadTelegramCredentials(c.Request.Context(), db, cfg)
		c.JSON(http.StatusOK, gin.H{
			"configured": credentials.APIID > 0 && credentials.APIHash != "",
			"apiId":      credentials.APIID,
			"hasApiHash": credentials.APIHash != "",
			"source":     credentials.Source,
		})
	})
	group.PUT("/telegram/settings", func(c *gin.Context) {
		var input struct {
			APIID   int    `json:"apiId"`
			APIHash string `json:"apiHash"`
		}
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请输入 Telegram API ID 和 API Hash"})
			return
		}
		input.APIHash = strings.TrimSpace(input.APIHash)
		if input.APIID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram API ID 必须是正整数"})
			return
		}
		if !telegramAPIHashPattern.MatchString(input.APIHash) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram API Hash 应为 32 位十六进制字符串"})
			return
		}
		encrypted, err := encryptAccountSecret(cfg.SessionKey, 0, telegramAPIHashPurpose, input.APIHash)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "加密 Telegram API Hash 失败"})
			return
		}
		tx, err := db.BeginTx(c.Request.Context(), nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 Telegram 配置失败"})
			return
		}
		defer tx.Rollback()
		for key, value := range map[string]string{
			telegramAPIIDSetting:   strconv.Itoa(input.APIID),
			telegramAPIHashSetting: base64.StdEncoding.EncodeToString(encrypted),
		} {
			if _, err := tx.ExecContext(c.Request.Context(), `
				INSERT INTO settings(key, value) VALUES(?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
			`, key, value); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 Telegram 配置失败"})
				return
			}
		}
		if err := tx.Commit(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 Telegram 配置失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true, "configured": true})
	})
}

func loadTelegramCredentials(ctx context.Context, db *sql.DB, cfg config.Config) telegramCredentials {
	credentials := telegramCredentials{APIID: cfg.TelegramAPIID, APIHash: cfg.TelegramAPIHash, Source: "environment"}
	rows, err := db.QueryContext(ctx, "SELECT key, value FROM settings WHERE key IN (?, ?)", telegramAPIIDSetting, telegramAPIHashSetting)
	if err != nil {
		return credentials
	}
	defer rows.Close()
	values := make(map[string]string, 2)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err == nil {
			values[key] = value
		}
	}
	apiID, idErr := strconv.Atoi(values[telegramAPIIDSetting])
	encrypted, decodeErr := base64.StdEncoding.DecodeString(values[telegramAPIHashSetting])
	apiHash, decryptErr := decryptAccountSecret(cfg.SessionKey, 0, telegramAPIHashPurpose, encrypted)
	if idErr == nil && apiID > 0 && decodeErr == nil && decryptErr == nil && telegramAPIHashPattern.MatchString(apiHash) {
		return telegramCredentials{APIID: apiID, APIHash: apiHash, Source: "saved"}
	}
	return credentials
}

func telegramCredentialsConfigured(credentials telegramCredentials) bool {
	return credentials.APIID > 0 && credentials.APIHash != ""
}
