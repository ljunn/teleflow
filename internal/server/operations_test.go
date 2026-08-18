package server

import (
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/database"
	"github.com/ljunn/teleflow/internal/updater"
)

func TestOperationsFlow(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "operations.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	handler := New(
		config.Config{PublicURL: "http://localhost"},
		db,
		updater.New(updater.Options{Current: "dev"}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	setup := serveJSON(handler, http.MethodPost, "/api/v1/auth/setup", `{"password":"admin1234"}`, nil)
	if setup.Code != http.StatusOK {
		t.Fatalf("setup returned %d: %s", setup.Code, setup.Body.String())
	}
	session := setup.Result().Cookies()[0]

	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/capabilities", "", session), http.StatusOK, `"telegramConfigured":false`)
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts", `{"phone":"+8613800138000","displayName":"获客账号 A"}`, session), http.StatusCreated, `"status":"pending"`)
	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/accounts", "", session), http.StatusOK, `"displayName":"获客账号 A"`)
	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/accounts", `{"phone":"+8613800138000"}`, session), http.StatusConflict, "该手机号已存在")

	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/discovery", `{"query":"web3","sourceType":"public_chat"}`, session), http.StatusCreated, `"status":"pending_connection"`)
	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/discovery", "", session), http.StatusOK, `"query":"web3"`)

	assertResponse(t, serveJSON(handler, http.MethodPost, "/api/v1/campaigns", `{"name":"首轮触达","kind":"direct_message","target":"web3 leads","message":"你好"}`, session), http.StatusCreated, `"status":"draft"`)
	assertResponse(t, serveJSON(handler, http.MethodPatch, "/api/v1/campaigns/1/status", `{"status":"pending_connection"}`, session), http.StatusOK, `"status":"pending_connection"`)
	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/campaigns", "", session), http.StatusOK, `"name":"首轮触达"`)
	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/overview", "", session), http.StatusOK, `"pendingJobs":2`)

	assertResponse(t, serveJSON(handler, http.MethodPut, "/api/v1/relay", `{"botUsername":"teleflow_relay_bot","masterUsername":"owner","enabled":true}`, session), http.StatusOK, `"ok":true`)
	assertResponse(t, serveJSON(handler, http.MethodGet, "/api/v1/relay", "", session), http.StatusOK, `"masterUsername":"owner"`)

	assertResponse(t, serveJSON(handler, http.MethodDelete, "/api/v1/accounts/1", "", session), http.StatusOK, `"ok":true`)
	assertResponse(t, serveJSON(handler, http.MethodDelete, "/api/v1/discovery/1", "", session), http.StatusOK, `"ok":true`)
	assertResponse(t, serveJSON(handler, http.MethodDelete, "/api/v1/campaigns/1", "", session), http.StatusOK, `"ok":true`)
}

func assertResponse(t *testing.T, response interface {
	Result() *http.Response
}, wantStatus int, wantBody string) {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		t.Fatal(err)
	}
	if result.StatusCode != wantStatus || !strings.Contains(string(contents), wantBody) {
		t.Fatalf("returned %d: %s; want status %d containing %q", result.StatusCode, contents, wantStatus, wantBody)
	}
}
