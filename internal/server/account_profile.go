package server

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gotd/td/tgerr"
)

const (
	maxProfileImageBytes  = 5 << 20
	maxProfileImagePixels = 16_000_000
)

type telegramProfile struct {
	DisplayName string `json:"displayName"`
	Bio         string `json:"bio"`
	Username    string `json:"username"`
	HasPhoto    bool   `json:"hasPhoto"`
}

func (s *operationsService) getAccountProfile(c *gin.Context) {
	id, ok := s.profileAccountID(c)
	if !ok {
		return
	}
	profile, err := s.authorizer.Profile(c.Request.Context(), id)
	if err != nil {
		s.writeProfileError(c, id, err)
		return
	}
	s.syncAccountProfile(c.Request.Context(), id, profile)
	c.JSON(http.StatusOK, profile)
}

func (s *operationsService) updateAccountProfile(c *gin.Context) {
	id, ok := s.profileAccountID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxProfileImageBytes+(1<<20))
	if err := c.Request.ParseMultipartForm(maxProfileImageBytes); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "资料或头像文件过大"})
		return
	}
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}

	displayName := strings.TrimSpace(c.PostForm("displayName"))
	bio := strings.TrimSpace(c.PostForm("bio"))
	if displayName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram 昵称不能为空"})
		return
	}
	if len([]rune(displayName)) > 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram 昵称不能超过 64 个字符"})
		return
	}
	if len([]rune(bio)) > 70 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Telegram 简介不能超过 70 个字符"})
		return
	}

	photo, err := profilePhotoFromRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	profile, err := s.authorizer.UpdateProfile(c.Request.Context(), id, displayName, bio, photo)
	if err != nil {
		s.writeProfileError(c, id, err)
		return
	}
	s.syncAccountProfile(c.Request.Context(), id, profile)
	c.JSON(http.StatusOK, profile)
}

func (s *operationsService) profileAccountID(c *gin.Context) (int64, bool) {
	id, ok := parseID(c)
	if !ok {
		return 0, false
	}
	var hasSession bool
	var status string
	err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0, status
		FROM telegram_accounts WHERE id = ?
	`, id).Scan(&hasSession, &status)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return 0, false
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号登录状态失败"})
		return 0, false
	}
	if !hasSession || status == "unauthorized" {
		c.JSON(http.StatusConflict, gin.H{"error": "账号尚未登录或 Telegram 会话已失效"})
		return 0, false
	}
	if status == "logging_in" || status == "checking" {
		c.JSON(http.StatusConflict, gin.H{"error": "账号正在执行连接操作，请稍后再试"})
		return 0, false
	}
	return id, true
}

func profilePhotoFromRequest(c *gin.Context) ([]byte, error) {
	header, err := c.FormFile("avatar")
	if errors.Is(err, http.ErrMissingFile) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.New("读取头像文件失败")
	}
	if header.Size <= 0 || header.Size > maxProfileImageBytes {
		return nil, errors.New("头像文件不能超过 5 MB")
	}
	file, err := header.Open()
	if err != nil {
		return nil, errors.New("打开头像文件失败")
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, maxProfileImageBytes+1))
	if err != nil || len(raw) > maxProfileImageBytes {
		return nil, errors.New("读取头像文件失败")
	}
	return normalizeProfilePhoto(raw)
}

func normalizeProfilePhoto(raw []byte) ([]byte, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") {
		return nil, errors.New("头像仅支持 JPG 或 PNG 图片")
	}
	if config.Width < 100 || config.Height < 100 {
		return nil, errors.New("头像尺寸不能小于 100 x 100 像素")
	}
	if config.Width > 4096 || config.Height > 4096 || config.Width*config.Height > maxProfileImagePixels {
		return nil, errors.New("头像尺寸不能超过 4096 x 4096 像素")
	}
	decoded, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("头像图片无法解析")
	}
	bounds := decoded.Bounds()
	normalized := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(normalized, normalized.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(normalized, normalized.Bounds(), decoded, bounds.Min, draw.Over)
	var output bytes.Buffer
	if err := jpeg.Encode(&output, normalized, &jpeg.Options{Quality: 90}); err != nil {
		return nil, errors.New("头像图片处理失败")
	}
	return output.Bytes(), nil
}

func (s *operationsService) syncAccountProfile(ctx context.Context, id int64, profile telegramProfile) {
	_, _ = s.db.ExecContext(ctx, `
		UPDATE telegram_accounts
		SET display_name = ?, username = ?, last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, profile.DisplayName, profile.Username, id)
}

func (s *operationsService) writeProfileError(c *gin.Context, id int64, err error) {
	message := telegramProfileErrorMessage(err)
	statusCode := http.StatusBadGateway
	status := telegramFailureStatus(err)
	if errors.Is(err, errTelegramNotConfigured) {
		statusCode = http.StatusServiceUnavailable
	}
	if errors.Is(err, errAccountUnauthorized) || status == "unauthorized" {
		statusCode = http.StatusConflict
		status = "unauthorized"
	}
	_, _ = s.db.ExecContext(c.Request.Context(), `
		UPDATE telegram_accounts
		SET status = CASE WHEN ? = 'unauthorized' THEN 'unauthorized' ELSE status END,
			last_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, id)
	c.JSON(statusCode, gin.H{"error": message})
}

func telegramProfileErrorMessage(err error) string {
	switch {
	case errors.Is(err, errTelegramNotConfigured):
		return "请先配置 Telegram API ID 和 API Hash"
	case errors.Is(err, errAccountUnauthorized):
		return "Telegram 会话已失效，请重新登录"
	case tgerr.Is(err, "FIRSTNAME_INVALID"):
		return "Telegram 不接受这个昵称"
	case tgerr.Is(err, "ABOUT_TOO_LONG"):
		return "Telegram 简介过长"
	case tgerr.Is(err, "PHOTO_EXT_INVALID", "PHOTO_CROP_SIZE_SMALL", "IMAGE_PROCESS_FAILED"):
		return "Telegram 无法处理这张头像，请更换图片"
	case tgerr.Is(err, "AUTH_KEY_UNREGISTERED", "SESSION_EXPIRED", "AUTH_KEY_DUPLICATED"):
		return "Telegram 会话已失效，请重新登录"
	case tgerr.Is(err, "FLOOD_WAIT"):
		return "Telegram 请求过于频繁，请稍后再试"
	case errors.Is(err, context.DeadlineExceeded):
		return "连接 Telegram 超时，请稍后再试"
	default:
		return "修改 Telegram 资料失败，请稍后重试"
	}
}
