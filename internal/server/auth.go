package server

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	passwordSetting = "admin_password_hash"
	sessionSetting  = "admin_session_hash"
	expiresSetting  = "admin_session_expires"
	sessionCookie   = "teleflow_session"
	sessionTTL      = 24 * time.Hour
)

type authService struct {
	db *sql.DB
}

func newAuthService(db *sql.DB) *authService {
	return &authService{db: db}
}

func (a *authService) configured(c *gin.Context) (bool, error) {
	var hash string
	err := a.db.QueryRowContext(c.Request.Context(), "SELECT value FROM settings WHERE key = ?", passwordSetting).Scan(&hash)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return hash != "", nil
}

func (a *authService) status(c *gin.Context) {
	configured, err := a.configured(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取认证状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"configured":    configured,
		"authenticated": a.authenticated(c),
	})
}

func (a *authService) setup(c *gin.Context) {
	configured, err := a.configured(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取认证状态失败"})
		return
	}
	if configured {
		c.JSON(http.StatusConflict, gin.H{"error": "管理员密码已设置"})
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || len([]rune(input.Password)) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码至少需要 8 个字符"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "设置密码失败"})
		return
	}
	result, err := a.db.ExecContext(c.Request.Context(), "INSERT OR IGNORE INTO settings(key, value) VALUES(?, ?)", passwordSetting, string(hash))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存密码失败"})
		return
	}
	if n, _ := result.RowsAffected(); n == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "管理员密码已设置"})
		return
	}
	if err := a.issueSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建登录会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *authService) login(c *gin.Context) {
	var input struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入密码"})
		return
	}
	var hash string
	if err := a.db.QueryRowContext(c.Request.Context(), "SELECT value FROM settings WHERE key = ?", passwordSetting).Scan(&hash); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "请先完成初始化"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取认证配置失败"})
		}
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "密码错误"})
		return
	}
	if err := a.issueSession(c); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建登录会话失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *authService) logout(c *gin.Context) {
	_, _ = a.db.ExecContext(c.Request.Context(), "DELETE FROM settings WHERE key IN (?, ?)", sessionSetting, expiresSetting)
	http.SetCookie(c.Writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (a *authService) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		configured, err := a.configured(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "读取认证状态失败"})
			return
		}
		if !configured || a.authenticated(c) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
	}
}

func (a *authService) authenticated(c *gin.Context) bool {
	token, err := c.Cookie(sessionCookie)
	if err != nil || strings.TrimSpace(token) == "" {
		return false
	}
	var storedHash, expiresValue string
	if err := a.db.QueryRowContext(c.Request.Context(), `
		SELECT
			(SELECT value FROM settings WHERE key = ?),
			(SELECT value FROM settings WHERE key = ?)
	`, sessionSetting, expiresSetting).Scan(&storedHash, &expiresValue); err != nil {
		return false
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresValue)
	if err != nil || time.Now().After(expires) {
		return false
	}
	tokenHash := sha256.Sum256([]byte(token))
	storedBytes, err := hex.DecodeString(storedHash)
	if err != nil || len(storedBytes) != len(tokenHash) {
		return false
	}
	return subtle.ConstantTimeCompare(tokenHash[:], storedBytes) == 1
}

func (a *authService) issueSession(c *gin.Context) error {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return errors.New("generate session token")
	}
	token := hex.EncodeToString(bytes[:])
	tokenHash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(sessionTTL)
	transaction, err := a.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		return fmt.Errorf("begin session transaction: %w", err)
	}
	defer transaction.Rollback()
	for key, value := range map[string]string{
		sessionSetting: hex.EncodeToString(tokenHash[:]),
		expiresSetting: expires.Format(time.RFC3339Nano),
	} {
		if _, err := transaction.ExecContext(c.Request.Context(), `
			INSERT INTO settings(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP
		`, key, value); err != nil {
			return fmt.Errorf("store session: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit session: %w", err)
	}
	secure := c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
	http.SetCookie(c.Writer, &http.Cookie{
		Name: sessionCookie, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secure,
	})
	return nil
}
