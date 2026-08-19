package server

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/database"
)

type fakeAccountAuthorizer struct {
	requestResult  accountAuthResult
	verifyResult   accountAuthResult
	passwordResult accountAuthResult
	autoResult     accountAuthResult
	checkResult    accountAuthResult
	phone          string
	codeHash       string
	code           string
	password       string
}

func (f *fakeAccountAuthorizer) RequestCode(_ context.Context, _ int64, phone string) (accountAuthResult, error) {
	f.phone = phone
	return f.requestResult, nil
}

func (f *fakeAccountAuthorizer) VerifyCode(_ context.Context, _ int64, phone, codeHash, code string) (accountAuthResult, error) {
	f.phone, f.codeHash, f.code = phone, codeHash, code
	return f.verifyResult, nil
}

func (f *fakeAccountAuthorizer) VerifyPassword(_ context.Context, _ int64, password string) (accountAuthResult, error) {
	f.password = password
	return f.passwordResult, nil
}

func (f *fakeAccountAuthorizer) AutoLogin(_ context.Context, _ int64, phone, _ string) (accountAuthResult, error) {
	f.phone = phone
	return f.autoResult, nil
}

func (f *fakeAccountAuthorizer) Check(_ context.Context, _ int64) (accountAuthResult, error) {
	return f.checkResult, nil
}

func TestAccountAuthorizationFlow(t *testing.T) {
	db := openTelegramTestDB(t)
	authorizer := &fakeAccountAuthorizer{
		requestResult:  accountAuthResult{Status: "code_sent", CodeHash: "telegram-code-hash"},
		verifyResult:   accountAuthResult{Status: "password_required"},
		passwordResult: accountAuthResult{Status: "authorized", UserID: 42, Username: "teleflow_owner", DisplayName: "Teleflow Owner"},
	}
	handler := telegramOperationsHandler(db, authorizer)

	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts", `{"phone":"+8613800138000","displayName":""}`, nil), http.StatusCreated, `"status":"pending"`)
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts/1/auth/code", `{}`, nil), http.StatusOK, `"status":"code_sent"`)
	assertAccountAuthState(t, db, 1, "code_sent", "telegram-code-hash")

	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts/1/auth/verify", `{"code":"12345"}`, nil), http.StatusOK, `"status":"password_required"`)
	assertAccountAuthState(t, db, 1, "password_required", "telegram-code-hash")

	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts/1/auth/password", `{"password":"two-factor-secret"}`, nil), http.StatusOK, `"status":"authorized"`)
	response := serveJSON(handler, http.MethodGet, "/api/v1/accounts", "", nil)
	assertResponse(t, response, http.StatusOK, `"username":"teleflow_owner"`)

	if authorizer.phone != "+8613800138000" || authorizer.codeHash != "telegram-code-hash" || authorizer.code != "12345" || authorizer.password != "two-factor-secret" {
		t.Fatalf("unexpected authorization inputs: %+v", authorizer)
	}
}

func TestAccountAuthorizationRequiresCorrectState(t *testing.T) {
	db := openTelegramTestDB(t)
	handler := telegramOperationsHandler(db, &fakeAccountAuthorizer{})
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts", `{"phone":"+8613800138001"}`, nil), http.StatusCreated, `"status":"pending"`)
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts/1/auth/verify", `{"code":"12345"}`, nil), http.StatusConflict, "请先发送验证码")
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts/1/auth/password", `{"password":"secret"}`, nil), http.StatusConflict, "当前不需要两步验证密码")
}

func TestImportQueuesAutoLoginWhenTelegramIsConfigured(t *testing.T) {
	db := openTelegramTestDB(t)
	authorizer := &fakeAccountAuthorizer{autoResult: accountAuthResult{Status: "authorized", UserID: 7, Username: "imported_user", DisplayName: "Imported User"}}
	key := bytes.Repeat([]byte{0x2a}, 32)
	handler := telegramOperationsHandlerWithConfig(db, authorizer, config.Config{TelegramAPIID: 123, TelegramAPIHash: "hash", SessionKey: key})

	response := serveJSON(handler, http.MethodPost, "/api/v1/accounts/import", `{"text":"+8613800138003----https://vendor.example/code"}`, nil)
	assertResponse(t, response, http.StatusOK, `"autoLoginQueued":1`)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		if err := db.QueryRow("SELECT status FROM telegram_accounts WHERE id = 1").Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status == "online" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("imported account did not finish automatic login")
}

func TestImportExplainsMissingTelegramConfiguration(t *testing.T) {
	db := openTelegramTestDB(t)
	handler := telegramOperationsHandlerWithConfig(db, &fakeAccountAuthorizer{}, config.Config{SessionKey: bytes.Repeat([]byte{0x2a}, 32)})

	response := serveJSON(handler, http.MethodPost, "/api/v1/accounts/import", `{"text":"+8613800138004----https://vendor.example/code"}`, nil)
	assertResponse(t, response, http.StatusOK, `"autoLoginBlocked":true`)
	var lastError string
	if err := db.QueryRow("SELECT last_error FROM telegram_accounts WHERE id = 1").Scan(&lastError); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(lastError, "未配置 Telegram API") {
		t.Fatalf("last_error = %q", lastError)
	}
}

func TestSavedTelegramSettingsEnableAutomaticLogin(t *testing.T) {
	db := openTelegramTestDB(t)
	authorizer := &fakeAccountAuthorizer{autoResult: accountAuthResult{Status: "authorized", UserID: 8}}
	cfg := config.Config{SessionKey: bytes.Repeat([]byte{0x2a}, 32)}
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	group := router.Group("/api/v1")
	registerTelegramSettingsRoutes(group, cfg, db)
	registerOperationsRoutes(group, cfg, db, authorizer)

	assertResponse(t, serveJSON(router, http.MethodPut, "/api/v1/telegram/settings", `{"apiId":123456,"apiHash":"0123456789abcdef0123456789abcdef"}`, nil), http.StatusOK, `"configured":true`)
	settings := serveJSON(router, http.MethodGet, "/api/v1/telegram/settings", "", nil)
	assertResponse(t, settings, http.StatusOK, `"apiId":123456`)
	if strings.Contains(settings.Body.String(), "0123456789abcdef") {
		t.Fatal("Telegram API Hash was exposed by settings endpoint")
	}
	response := serveJSON(router, http.MethodPost, "/api/v1/accounts/import", `{"text":"+8613800138005----https://vendor.example/code"}`, nil)
	assertResponse(t, response, http.StatusOK, `"autoLoginQueued":1`)
}

func TestEncryptedSessionStorage(t *testing.T) {
	db := openTelegramTestDB(t)
	if _, err := db.Exec("INSERT INTO telegram_accounts(phone) VALUES('+8613800138002')"); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{0x2a}, 32)
	storage := &encryptedSessionStorage{db: db, accountID: 1, key: key}
	plain := []byte(`{"auth_key":"sensitive-session-data"}`)
	if err := storage.StoreSession(context.Background(), plain); err != nil {
		t.Fatal(err)
	}
	var encrypted []byte
	if err := db.QueryRow("SELECT session_ciphertext FROM telegram_accounts WHERE id = 1").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(encrypted, plain) || bytes.Contains(encrypted, []byte("sensitive-session-data")) {
		t.Fatal("session was stored without encryption")
	}
	loaded, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(loaded, plain) {
		t.Fatalf("loaded session differs: %q", loaded)
	}
	wrongKey := &encryptedSessionStorage{db: db, accountID: 1, key: bytes.Repeat([]byte{0x3b}, 32)}
	if _, err := wrongKey.LoadSession(context.Background()); err == nil {
		t.Fatal("expected decryption with a wrong key to fail")
	}
}

func openTelegramTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "telegram-auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func telegramOperationsHandler(db *sql.DB, authorizer accountAuthorizer) http.Handler {
	return telegramOperationsHandlerWithConfig(db, authorizer, config.Config{})
}

func telegramOperationsHandlerWithConfig(db *sql.DB, authorizer accountAuthorizer, cfg config.Config) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	group := router.Group("/api/v1")
	registerOperationsRoutes(group, cfg, db, authorizer)
	return router
}

func assertAccountAuthState(t *testing.T, db *sql.DB, id int64, wantStatus, wantHash string) {
	t.Helper()
	var status, codeHash string
	if err := db.QueryRow("SELECT status, auth_code_hash FROM telegram_accounts WHERE id = ?", id).Scan(&status, &codeHash); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || codeHash != wantHash {
		t.Fatalf("account state = %q, %q; want %q, %q", status, codeHash, wantStatus, wantHash)
	}
}
