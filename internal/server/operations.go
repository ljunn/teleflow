package server

import (
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
)

var phonePattern = regexp.MustCompile(`^\+?[0-9]{7,15}$`)

type operationsService struct {
	db  *sql.DB
	cfg config.Config
}

type telegramAccount struct {
	ID          int64   `json:"id"`
	Phone       string  `json:"phone"`
	DisplayName string  `json:"displayName"`
	Status      string  `json:"status"`
	LastSeenAt  *string `json:"lastSeenAt"`
	CreatedAt   string  `json:"createdAt"`
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

func registerOperationsRoutes(group *gin.RouterGroup, cfg config.Config, db *sql.DB) {
	service := &operationsService{db: db, cfg: cfg}
	group.GET("/capabilities", service.capabilities)
	group.GET("/accounts", service.listAccounts)
	group.POST("/accounts", service.createAccount)
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
	if err := s.db.QueryRowContext(c.Request.Context(), "SELECT COUNT(*) FROM telegram_accounts WHERE status = 'online'").Scan(&connectedAccounts); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取运行能力失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"telegramConfigured": s.cfg.TelegramAPIID > 0 && s.cfg.TelegramAPIHash != "",
		"relayBotConfigured": s.cfg.RelayBotToken != "",
		"connectedAccounts":  connectedAccounts,
	})
}

func (s *operationsService) listAccounts(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `
		SELECT id, phone, display_name, status, last_seen_at, created_at
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
		var lastSeen sql.NullString
		if err := rows.Scan(&item.ID, &item.Phone, &item.DisplayName, &item.Status, &lastSeen, &item.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "读取账号列表失败"})
			return
		}
		if lastSeen.Valid {
			item.LastSeenAt = &lastSeen.String
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

func (s *operationsService) deleteAccount(c *gin.Context) {
	s.deleteByID(c, "telegram_accounts", "账号")
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
