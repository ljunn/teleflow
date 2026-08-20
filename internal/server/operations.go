package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
)

var phonePattern = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

type operationsService struct {
	db         *sql.DB
	cfg        config.Config
	authorizer accountAuthorizer
}

type telegramAccount struct {
	ID            int64   `json:"id"`
	Phone         string  `json:"phone"`
	DisplayName   string  `json:"displayName"`
	Remark        string  `json:"remark"`
	Status        string  `json:"status"`
	Username      string  `json:"username"`
	LastError     string  `json:"lastError"`
	HasCodeURL    bool    `json:"hasCodeUrl"`
	HasSession    bool    `json:"hasSession"`
	SourceType    string  `json:"sourceType"`
	LastSeenAt    *string `json:"lastSeenAt"`
	LastCheckedAt *string `json:"lastCheckedAt"`
	CodeSentAt    *string `json:"codeSentAt"`
	CreatedAt     string  `json:"createdAt"`
}

type accountImportError struct {
	Line  int    `json:"line"`
	Error string `json:"error"`
}

type discoveryTask struct {
	ID          int64  `json:"id"`
	Query       string `json:"query"`
	SourceType  string `json:"sourceType"`
	Status      string `json:"status"`
	ResultCount int    `json:"resultCount"`
	LastError   string `json:"lastError"`
	CreatedAt   string `json:"createdAt"`
}

type campaign struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Target      string  `json:"target"`
	Message     string  `json:"message"`
	Status      string  `json:"status"`
	RunAt       *string `json:"runAt"`
	SentCount   int     `json:"sentCount"`
	FailedCount int     `json:"failedCount"`
	LastError   string  `json:"lastError"`
	CreatedAt   string  `json:"createdAt"`
}

type relaySettings struct {
	BotUsername    string `json:"botUsername"`
	MasterUsername string `json:"masterUsername"`
	Enabled        bool   `json:"enabled"`
	UpdatedAt      string `json:"updatedAt"`
}

func registerOperationsRoutes(group *gin.RouterGroup, cfg config.Config, db *sql.DB, authorizers ...accountAuthorizer) {
	authorizer := newGotdAccountAuthorizer(cfg, db)
	if len(authorizers) > 0 && authorizers[0] != nil {
		authorizer = authorizers[0]
	}
	service := &operationsService{db: db, cfg: cfg, authorizer: authorizer}
	_, _ = db.Exec(`
		UPDATE telegram_accounts
		SET status = CASE WHEN session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0 THEN 'online' ELSE 'error' END,
			last_error = CASE WHEN session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0 THEN '' ELSE '上次连接操作因服务重启而中断，请重试' END,
			updated_at = CURRENT_TIMESTAMP
		WHERE status IN ('logging_in', 'checking')
	`)
	if !service.telegramConfigured(context.Background()) {
		_, _ = db.Exec(`
			UPDATE telegram_accounts
			SET last_error = '未配置 Telegram API ID 和 API Hash，无法自动登录', updated_at = CURRENT_TIMESTAMP
			WHERE status = 'pending' AND code_url_ciphertext IS NOT NULL AND length(code_url_ciphertext) > 0
		`)
	}
	group.GET("/capabilities", service.capabilities)
	group.GET("/accounts", service.listAccounts)
	group.POST("/accounts", service.createAccount)
	group.PUT("/accounts/:id", service.updateAccount)
	group.POST("/accounts/import", service.importAccounts)
	group.POST("/accounts/check", service.checkAllAccounts)
	group.POST("/accounts/:id/auth/code", service.requestAccountCode)
	group.POST("/accounts/:id/auth/verify", service.verifyAccountCode)
	group.POST("/accounts/:id/auth/password", service.verifyAccountPassword)
	group.POST("/accounts/:id/auth/auto", service.autoLoginAccount)
	group.POST("/accounts/:id/check", service.checkAccount)
	group.DELETE("/accounts/:id", service.deleteAccount)
	group.GET("/discovery", service.listDiscovery)
	group.POST("/discovery", service.createDiscovery)
	group.DELETE("/discovery/:id", service.deleteDiscovery)
	group.GET("/campaigns", service.listCampaigns)
	group.POST("/campaigns", service.createCampaign)
	group.PATCH("/campaigns/:id/status", service.updateCampaignStatus)
	group.DELETE("/campaigns/:id", service.deleteCampaign)
	group.GET("/relay", service.getRelaySettings)
	group.PUT("/relay", service.updateRelaySettings)
}

func (s *operationsService) capabilities(c *gin.Context) {
	var connectedAccounts int
	if err := s.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM telegram_accounts WHERE status IN ('online', 'authorized', 'restricted')").Scan(&connectedAccounts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取运行能力失败"})
		return
	}
	credentials := loadTelegramCredentials(c.Request.Context(), s.db, s.cfg)
	c.JSON(http.StatusOK, gin.H{
		"telegramConfigured": telegramCredentialsConfigured(credentials),
		"relayBotConfigured": s.cfg.RelayBotToken != "",
		"connectedAccounts":  connectedAccounts,
	})
}

func (s *operationsService) listAccounts(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
			SELECT id, phone, display_name, remark, status, username, last_error,
				code_url_ciphertext IS NOT NULL,
				session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0,
				source_type, last_seen_at, last_checked_at,
				auth_code_sent_at, created_at
			FROM telegram_accounts ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号列表失败"})
		return
	}
	defer rows.Close()
	accounts := make([]telegramAccount, 0)
	for rows.Next() {
		var item telegramAccount
		var lastSeen, lastChecked, codeSent sql.NullString
		if err := rows.Scan(&item.ID, &item.Phone, &item.DisplayName, &item.Remark, &item.Status, &item.Username, &item.LastError, &item.HasCodeURL, &item.HasSession, &item.SourceType, &lastSeen, &lastChecked, &codeSent, &item.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号列表失败"})
			return
		}
		if lastSeen.Valid {
			item.LastSeenAt = &lastSeen.String
		}
		if lastChecked.Valid {
			item.LastCheckedAt = &lastChecked.String
		}
		if codeSent.Valid {
			item.CodeSentAt = &codeSent.String
		}
		accounts = append(accounts, item)
	}
	c.JSON(http.StatusOK, accounts)
}

func (s *operationsService) createAccount(c *gin.Context) {
	var input struct {
		Phone       string `json:"phone"`
		DisplayName string `json:"displayName"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账号信息"})
		return
	}
	input.Phone = strings.ReplaceAll(strings.TrimSpace(input.Phone), " ", "")
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if !phonePattern.MatchString(input.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "手机号格式不正确"})
		return
	}
	if len([]rune(input.DisplayName)) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号名称不能超过 80 个字符"})
		return
	}
	result, err := s.db.ExecContext(c.Request.Context(), `
		INSERT INTO telegram_accounts(phone, display_name, status) VALUES(?, ?, 'pending')
	`, input.Phone, input.DisplayName)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "该手机号已存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存账号失败"})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "pending"})
}

func (s *operationsService) updateAccount(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input struct {
		DisplayName string `json:"displayName"`
		Remark      string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入账号昵称和备注"})
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Remark = strings.TrimSpace(input.Remark)
	if len([]rune(input.DisplayName)) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号昵称不能超过 80 个字符"})
		return
	}
	if len([]rune(input.Remark)) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "账号备注不能超过 500 个字符"})
		return
	}
	result, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE telegram_accounts
		SET display_name = ?, remark = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, input.DisplayName, input.Remark, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存账号信息失败"})
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *operationsService) importAccounts(c *gin.Context) {
	var input struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Text) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请粘贴需要导入的账号"})
		return
	}
	if len(input.Text) > 256*1024 {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "单次导入内容不能超过 256 KiB"})
		return
	}
	if len(s.cfg.SessionKey) != 32 {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "会话加密密钥不可用"})
		return
	}
	items, problems := parseAccountImport(input.Text)
	if len(items) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "单次最多导入 500 个账号"})
		return
	}
	if len(items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "没有识别到可导入的账号", "errors": problems})
		return
	}

	tx, err := s.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "开始导入失败"})
		return
	}
	defer tx.Rollback()
	added, updated, skipped := 0, 0, 0
	importedIDs := make([]int64, 0, len(items))
	for _, item := range items {
		var accountID int64
		var displayName string
		var currentCiphertext []byte
		isNew := false
		err := tx.QueryRowContext(c.Request.Context(), `
			SELECT id, display_name, code_url_ciphertext FROM telegram_accounts WHERE phone = ?
		`, item.Phone).Scan(&accountID, &displayName, &currentCiphertext)
		if errors.Is(err, sql.ErrNoRows) {
			result, insertErr := tx.ExecContext(c.Request.Context(), `
				INSERT INTO telegram_accounts(phone, display_name, status, source_type)
				VALUES(?, ?, 'pending', 'code_url')
			`, item.Phone, item.DisplayName)
			if insertErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "保存导入账号失败"})
				return
			}
			accountID, _ = result.LastInsertId()
			isNew = true
			added++
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取现有账号失败"})
			return
		}

		changed := len(currentCiphertext) == 0
		if len(currentCiphertext) > 0 {
			currentURL, decryptErr := decryptAccountSecret(s.cfg.SessionKey, accountID, "code-url", currentCiphertext)
			changed = decryptErr != nil || currentURL != item.CodeURL
		}
		nameChanged := displayName == "" && item.DisplayName != ""
		if accountID == 0 {
			// Defensive only; LastInsertId is required by SQLite.
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存导入账号失败"})
			return
		}
		encrypted, encryptErr := encryptAccountSecret(s.cfg.SessionKey, accountID, "code-url", item.CodeURL)
		if encryptErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "加密取码链接失败"})
			return
		}
		if _, err := tx.ExecContext(c.Request.Context(), `
			UPDATE telegram_accounts
			SET code_url_ciphertext = ?, source_type = 'code_url',
				display_name = CASE WHEN display_name = '' THEN ? ELSE display_name END,
				updated_at = CURRENT_TIMESTAMP
			WHERE id = ?
		`, encrypted, item.DisplayName, accountID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存取码链接失败"})
			return
		}
		if isNew {
			importedIDs = append(importedIDs, accountID)
			continue
		}
		importedIDs = append(importedIDs, accountID)
		if changed || nameChanged {
			updated++
		} else {
			skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "提交导入失败"})
		return
	}
	autoLoginQueued := 0
	autoLoginBlocked := false
	if s.telegramConfigured(c.Request.Context()) {
		for _, accountID := range importedIDs {
			queued, queueErr := s.queueAutoLogin(c.Request.Context(), accountID)
			if queueErr != nil {
				if errors.Is(queueErr, errAccountBusy) {
					continue
				}
				writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, _ = s.db.ExecContext(writeCtx, "UPDATE telegram_accounts SET last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", telegramErrorMessage(queueErr), accountID)
				cancel()
			} else if queued {
				autoLoginQueued++
			}
		}
	} else {
		autoLoginBlocked = true
		for _, accountID := range importedIDs {
			_, _ = s.db.ExecContext(c.Request.Context(), `
				UPDATE telegram_accounts
				SET last_error = '已导入，但未配置 Telegram API ID 和 API Hash，无法自动登录', updated_at = CURRENT_TIMESTAMP
				WHERE id = ? AND (session_ciphertext IS NULL OR length(session_ciphertext) = 0)
			`, accountID)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"parsed": len(items), "added": added, "updated": updated,
		"skipped": skipped, "errors": problems,
		"autoLoginQueued": autoLoginQueued, "autoLoginBlocked": autoLoginBlocked,
	})
}

func (s *operationsService) telegramConfigured(ctx context.Context) bool {
	return telegramCredentialsConfigured(loadTelegramCredentials(ctx, s.db, s.cfg))
}

func (s *operationsService) autoLoginAccount(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	if !s.telegramConfigured(c.Request.Context()) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "请先配置 Telegram API ID 和 API Hash"})
		return
	}
	queued, err := s.queueAutoLogin(c.Request.Context(), id)
	if errors.Is(err, errTelegramNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "请先配置 Telegram API ID 和 API Hash"})
		return
	}
	if errors.Is(err, errAccountBusy) {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号正在执行连接操作"})
		return
	}
	if errors.Is(err, errAccountNoCodeURL) {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号没有取码链接，请使用验证码登录"})
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": telegramErrorMessage(err)})
		return
	}
	if !queued {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号当前已有会话或正在执行连接操作"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "logging_in"})
}

var (
	errAccountBusy      = errors.New("account connection is already running")
	errAccountNoCodeURL = errors.New("account has no code URL")
)

func (s *operationsService) queueAutoLogin(ctx context.Context, id int64) (bool, error) {
	if !s.telegramConfigured(ctx) {
		return false, errTelegramNotConfigured
	}
	var phone, status string
	var encrypted []byte
	var hasSession bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT phone, status, code_url_ciphertext, session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0 FROM telegram_accounts WHERE id = ?
	`, id).Scan(&phone, &status, &encrypted, &hasSession); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, sql.ErrNoRows
		} else {
			return false, err
		}
	}
	if len(encrypted) == 0 {
		return false, errAccountNoCodeURL
	}
	if hasSession && status != "unauthorized" {
		return false, nil
	}
	if status == "logging_in" || status == "checking" {
		return false, errAccountBusy
	}
	switch status {
	case "pending", "error", "unauthorized", "flood_wait", "offline":
	default:
		return false, nil
	}
	codeURL, err := decryptAccountSecret(s.cfg.SessionKey, id, "code-url", encrypted)
	if err != nil {
		return false, errors.New("读取取码链接失败")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE telegram_accounts
		SET status = 'logging_in',
			session_ciphertext = CASE WHEN status = 'unauthorized' THEN NULL ELSE session_ciphertext END,
			last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status IN ('pending', 'error', 'unauthorized', 'flood_wait', 'offline')
	`, id)
	if err != nil {
		return false, err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return false, errAccountBusy
	}
	go s.runAutoLogin(id, phone, codeURL)
	return true, nil
}

func (s *operationsService) runAutoLogin(id int64, phone, codeURL string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	result, err := s.authorizer.AutoLogin(ctx, id, phone, codeURL)
	if err != nil {
		s.storeBackgroundAccountError(ctx, id, err)
		return
	}
	if result.Status == "password_required" {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE telegram_accounts SET status = 'password_required', last_error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, id)
		return
	}
	if err := s.saveAccountAuthorization(ctx, id, result); err != nil {
		_, _ = s.db.ExecContext(ctx, `
			UPDATE telegram_accounts SET status = 'error', last_error = '保存 Telegram 授权失败', updated_at = CURRENT_TIMESTAMP WHERE id = ?
		`, id)
	}
}

func (s *operationsService) checkAccount(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	queued, err := s.queueAccountCheck(c.Request.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "启动状态检测失败"})
		return
	}
	if !queued {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号正在执行连接操作或尚未登录"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "checking"})
}

func (s *operationsService) checkAllAccounts(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id FROM telegram_accounts
		WHERE session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0
			AND status NOT IN ('logging_in', 'checking')
		ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取待检测账号失败"})
		return
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	queued := make([]int64, 0, len(ids))
	for _, id := range ids {
		if ok, _ := s.markAccountChecking(c.Request.Context(), id); ok {
			queued = append(queued, id)
		}
	}
	go func() {
		for _, id := range queued {
			s.runAccountCheck(id)
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"queued": len(queued)})
}

func (s *operationsService) queueAccountCheck(ctx context.Context, id int64) (bool, error) {
	var hasSession bool
	if err := s.db.QueryRowContext(ctx, `
		SELECT session_ciphertext IS NOT NULL AND length(session_ciphertext) > 0
		FROM telegram_accounts WHERE id = ?
	`, id).Scan(&hasSession); err != nil {
		return false, err
	}
	if !hasSession {
		return false, nil
	}
	queued, err := s.markAccountChecking(ctx, id)
	if err == nil && queued {
		go s.runAccountCheck(id)
	}
	return queued, err
}

func (s *operationsService) markAccountChecking(ctx context.Context, id int64) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE telegram_accounts SET status = 'checking', last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ? AND status NOT IN ('logging_in', 'checking')
	`, id)
	if err != nil {
		return false, err
	}
	rows, _ := result.RowsAffected()
	return rows > 0, nil
}

func (s *operationsService) runAccountCheck(id int64) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result, err := s.authorizer.Check(ctx, id)
	if err != nil {
		s.storeBackgroundAccountError(ctx, id, err)
		return
	}
	status := result.Status
	if status != "online" && status != "restricted" && status != "unauthorized" && status != "banned" {
		status = "error"
	}
	lastError := ""
	if status == "unauthorized" {
		lastError = "Telegram 会话已失效，请重新登录"
	} else if status == "banned" {
		lastError = "该 Telegram 账号已停用或封禁"
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE telegram_accounts
		SET status = ?, telegram_user_id = CASE WHEN ? > 0 THEN ? ELSE telegram_user_id END,
			username = CASE WHEN ? != '' THEN ? ELSE username END,
			display_name = CASE WHEN display_name = '' THEN ? ELSE display_name END,
			last_error = ?, last_checked_at = CURRENT_TIMESTAMP,
			last_seen_at = CASE WHEN ? IN ('online', 'restricted') THEN CURRENT_TIMESTAMP ELSE last_seen_at END,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, result.UserID, result.UserID, result.Username, result.Username, result.DisplayName, lastError, status, id)
}

func (s *operationsService) storeBackgroundAccountError(ctx context.Context, id int64, authErr error) {
	message := telegramErrorMessage(authErr)
	status := telegramFailureStatus(authErr)
	// The worker context may already be canceled because the Telegram timeout
	// elapsed. Persist the terminal state with a short independent context so
	// accounts cannot remain stuck in logging_in/checking forever.
	writeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _ = s.db.ExecContext(writeCtx, `
		UPDATE telegram_accounts
		SET status = ?, last_error = ?, last_checked_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, message, id)
}

func (s *operationsService) deleteAccount(c *gin.Context) {
	s.deleteByID(c, "telegram_accounts", "账号")
}

func (s *operationsService) requestAccountCode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	phone, _, status, ok := s.accountAuthState(c, id)
	if !ok {
		return
	}
	if status == "authorized" || status == "online" {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号已经完成授权"})
		return
	}
	result, err := s.authorizer.RequestCode(c.Request.Context(), id, phone)
	if err != nil {
		s.storeAccountAuthError(c, id, err)
		return
	}
	if result.Status == "authorized" {
		if !s.storeAccountAuthorization(c, id, result) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "authorized"})
		return
	}
	if result.Status != "code_sent" || result.CodeHash == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Telegram 未返回有效验证码状态"})
		return
	}
	if _, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE telegram_accounts
		SET status = 'code_sent', auth_code_hash = ?, auth_code_sent_at = CURRENT_TIMESTAMP,
			last_error = '', updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, result.CodeHash, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存验证码状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "code_sent"})
}

func (s *operationsService) verifyAccountCode(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入验证码"})
		return
	}
	input.Code = strings.TrimSpace(input.Code)
	if matched, _ := regexp.MatchString(`^[0-9]{3,8}$`, input.Code); !matched {
		c.JSON(http.StatusBadRequest, gin.H{"error": "验证码格式不正确"})
		return
	}
	phone, codeHash, status, ok := s.accountAuthState(c, id)
	if !ok {
		return
	}
	if status != "code_sent" || codeHash == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "请先发送验证码"})
		return
	}
	result, err := s.authorizer.VerifyCode(c.Request.Context(), id, phone, codeHash, input.Code)
	if err != nil {
		s.storeAccountAuthError(c, id, err)
		return
	}
	if result.Status == "password_required" {
		if _, err := s.db.ExecContext(c.Request.Context(), "UPDATE telegram_accounts SET status = 'password_required', last_error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "保存两步验证状态失败"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "password_required"})
		return
	}
	if !s.storeAccountAuthorization(c, id, result) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "authorized"})
}

func (s *operationsService) verifyAccountPassword(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.Password == "" || len(input.Password) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入有效的两步验证密码"})
		return
	}
	_, _, status, ok := s.accountAuthState(c, id)
	if !ok {
		return
	}
	if status != "password_required" {
		c.JSON(http.StatusConflict, gin.H{"error": "该账号当前不需要两步验证密码"})
		return
	}
	result, err := s.authorizer.VerifyPassword(c.Request.Context(), id, input.Password)
	if err != nil {
		s.storeAccountAuthError(c, id, err)
		return
	}
	if !s.storeAccountAuthorization(c, id, result) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "authorized"})
}

func (s *operationsService) accountAuthState(c *gin.Context, id int64) (phone, codeHash, status string, ok bool) {
	if err := s.db.QueryRowContext(c.Request.Context(), "SELECT phone, auth_code_hash, status FROM telegram_accounts WHERE id = ?", id).Scan(&phone, &codeHash, &status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "账号不存在"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号授权状态失败"})
		}
		return "", "", "", false
	}
	return phone, codeHash, status, true
}

func (s *operationsService) storeAccountAuthorization(c *gin.Context, id int64, result accountAuthResult) bool {
	if result.Status != "authorized" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Telegram 未返回有效授权状态"})
		return false
	}
	if err := s.saveAccountAuthorization(c.Request.Context(), id, result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存 Telegram 授权失败"})
		return false
	}
	return true
}

func (s *operationsService) saveAccountAuthorization(ctx context.Context, id int64, result accountAuthResult) error {
	if result.Status != "authorized" {
		return errors.New("invalid Telegram authorization result")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE telegram_accounts
		SET status = 'online', telegram_user_id = ?, username = ?,
			display_name = CASE WHEN display_name = '' THEN ? ELSE display_name END,
			auth_code_hash = '', auth_code_sent_at = NULL, last_error = '',
			last_seen_at = CURRENT_TIMESTAMP, last_checked_at = CURRENT_TIMESTAMP,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, result.UserID, result.Username, result.DisplayName, id)
	return err
}

func (s *operationsService) storeAccountAuthError(c *gin.Context, id int64, authErr error) {
	message := telegramErrorMessage(authErr)
	statusCode := http.StatusBadGateway
	if errors.Is(authErr, errTelegramNotConfigured) {
		statusCode = http.StatusServiceUnavailable
	}
	_, _ = s.db.ExecContext(c.Request.Context(), "UPDATE telegram_accounts SET last_error = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", message, id)
	c.JSON(statusCode, gin.H{"error": message})
}

func (s *operationsService) listDiscovery(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, query, source_type, status, result_count, last_error, created_at
		FROM discovery_tasks ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取采集任务失败"})
		return
	}
	defer rows.Close()
	items := make([]discoveryTask, 0)
	for rows.Next() {
		var item discoveryTask
		if err := rows.Scan(&item.ID, &item.Query, &item.SourceType, &item.Status, &item.ResultCount, &item.LastError, &item.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取采集任务失败"})
			return
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *operationsService) createDiscovery(c *gin.Context) {
	var input struct {
		Query      string `json:"query"`
		SourceType string `json:"sourceType"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入采集条件"})
		return
	}
	input.Query = strings.TrimSpace(input.Query)
	if len([]rune(input.Query)) < 2 || len([]rune(input.Query)) > 120 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "关键词长度需要在 2 到 120 个字符之间"})
		return
	}
	if input.SourceType != "public_chat" && input.SourceType != "public_channel" && input.SourceType != "message_history" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的采集类型"})
		return
	}
	status := "pending_connection"
	result, err := s.db.ExecContext(c.Request.Context(), `
		INSERT INTO discovery_tasks(query, source_type, status) VALUES(?, ?, ?)
	`, input.Query, input.SourceType, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建采集任务失败"})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": status})
}

func (s *operationsService) deleteDiscovery(c *gin.Context) {
	s.deleteByID(c, "discovery_tasks", "采集任务")
}

func (s *operationsService) listCampaigns(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, name, kind, target, message, status, run_at, sent_count, failed_count, last_error, created_at
		FROM campaigns ORDER BY created_at DESC, id DESC
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取营销任务失败"})
		return
	}
	defer rows.Close()
	items := make([]campaign, 0)
	for rows.Next() {
		var item campaign
		var runAt sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.Target, &item.Message, &item.Status, &runAt, &item.SentCount, &item.FailedCount, &item.LastError, &item.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取营销任务失败"})
			return
		}
		if runAt.Valid {
			item.RunAt = &runAt.String
		}
		items = append(items, item)
	}
	c.JSON(http.StatusOK, items)
}

func (s *operationsService) createCampaign(c *gin.Context) {
	var input struct {
		Name    string `json:"name"`
		Kind    string `json:"kind"`
		Target  string `json:"target"`
		Message string `json:"message"`
		RunAt   string `json:"runAt"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入营销任务"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Target = strings.TrimSpace(input.Target)
	input.Message = strings.TrimSpace(input.Message)
	if input.Name == "" || len([]rune(input.Name)) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "任务名称不能为空且不能超过 100 个字符"})
		return
	}
	if (input.Kind != "join_group" && input.Message == "") || len([]rune(input.Message)) > 4096 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "消息内容不能为空且不能超过 4096 个字符"})
		return
	}
	if input.Kind != "direct_message" && input.Kind != "group_message" && input.Kind != "join_group" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的任务类型"})
		return
	}
	result, err := s.db.ExecContext(c.Request.Context(), `
		INSERT INTO campaigns(name, kind, target, message, status, run_at)
		VALUES(?, ?, ?, ?, 'draft', NULLIF(?, ''))
	`, input.Name, input.Kind, input.Target, input.Message, strings.TrimSpace(input.RunAt))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建营销任务失败"})
		return
	}
	id, _ := result.LastInsertId()
	c.JSON(http.StatusCreated, gin.H{"id": id, "status": "draft"})
}

func (s *operationsService) updateCampaignStatus(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入任务状态"})
		return
	}
	allowed := map[string]bool{"draft": true, "paused": true, "pending_connection": true}
	if !allowed[input.Status] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "不支持的任务状态"})
		return
	}
	result, err := s.db.ExecContext(c.Request.Context(), "UPDATE campaigns SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", input.Status, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新营销任务失败"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "营销任务不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "status": input.Status})
}

func (s *operationsService) deleteCampaign(c *gin.Context) {
	s.deleteByID(c, "campaigns", "营销任务")
}

func (s *operationsService) getRelaySettings(c *gin.Context) {
	var item relaySettings
	var enabled int
	if err := s.db.QueryRowContext(c.Request.Context(), `
		SELECT bot_username, master_username, enabled, updated_at FROM relay_settings WHERE id = 1
	`).Scan(&item.BotUsername, &item.MasterUsername, &enabled, &item.UpdatedAt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取中转配置失败"})
		return
	}
	item.Enabled = enabled == 1
	c.JSON(http.StatusOK, item)
}

func (s *operationsService) updateRelaySettings(c *gin.Context) {
	var input relaySettings
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请输入中转配置"})
		return
	}
	input.BotUsername = strings.TrimPrefix(strings.TrimSpace(input.BotUsername), "@")
	input.MasterUsername = strings.TrimPrefix(strings.TrimSpace(input.MasterUsername), "@")
	if input.Enabled && (input.BotUsername == "" || input.MasterUsername == "") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "启用中转前需要填写 Bot 和主账号用户名"})
		return
	}
	_, err := s.db.ExecContext(c.Request.Context(), `
		UPDATE relay_settings
		SET bot_username = ?, master_username = ?, enabled = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = 1
	`, input.BotUsername, input.MasterUsername, boolInt(input.Enabled))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存中转配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (s *operationsService) deleteByID(c *gin.Context, table, label string) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	result, err := s.db.ExecContext(c.Request.Context(), "DELETE FROM "+table+" WHERE id = ?", id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除" + label + "失败"})
		return
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": label + "不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的 ID"})
		return 0, false
	}
	return id, true
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
