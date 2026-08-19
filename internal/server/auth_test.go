package server

import (
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/database"
	"github.com/ljunn/teleflow/internal/updater"
)

func TestAuthenticationFlow(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	handler := testHandler(db)

	response := serveJSON(handler, http.MethodGet, "/api/v1/auth/status", "", nil)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":false`) || !strings.Contains(response.Body.String(), `"defaultPassword":"admin"`) {
		t.Fatalf("initial status returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/setup", `{"password":"not-admin"}`, nil)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("custom setup password returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/setup", `{"password":"admin"}`, nil)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("setup returned %d: %s", response.Code, response.Body.String())
	}
	session := response.Result().Cookies()[0]

	response = serveJSON(handler, http.MethodGet, "/api/v1/overview", "", session)
	if response.Code != http.StatusOK {
		t.Fatalf("authenticated overview returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodGet, "/api/v1/auth/status", "", session)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authenticated":true`) {
		t.Fatalf("authenticated status returned %d: %s", response.Code, response.Body.String())
	}

	handler = testHandler(db)
	response = serveJSON(handler, http.MethodGet, "/api/v1/auth/status", "", session)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"authenticated":true`) {
		t.Fatalf("status after server restart returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodGet, "/api/v1/overview", "", nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous overview returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/login", `{"password":"incorrect"}`, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/login", `{"password":"admin"}`, nil)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("login returned %d: %s", response.Code, response.Body.String())
	}

	loginSession := response.Result().Cookies()[0]
	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"incorrect","newPassword":"new-admin-password","confirmPassword":"new-admin-password"}`, loginSession)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("wrong current password returned %d: %s", response.Code, response.Body.String())
	}
	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/password", `{"currentPassword":"admin","newPassword":"new-admin-password","confirmPassword":"new-admin-password"}`, loginSession)
	if response.Code != http.StatusOK || len(response.Result().Cookies()) != 1 {
		t.Fatalf("change password returned %d: %s", response.Code, response.Body.String())
	}
	loginSession = response.Result().Cookies()[0]
	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/login", `{"password":"admin"}`, nil)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("old password returned %d: %s", response.Code, response.Body.String())
	}
	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/login", `{"password":"new-admin-password"}`, nil)
	if response.Code != http.StatusOK {
		t.Fatalf("new password returned %d: %s", response.Code, response.Body.String())
	}

	response = serveJSON(handler, http.MethodPost, "/api/v1/auth/logout", "", loginSession)
	if response.Code != http.StatusOK {
		t.Fatalf("logout returned %d: %s", response.Code, response.Body.String())
	}
	response = serveJSON(handler, http.MethodGet, "/api/v1/overview", "", loginSession)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("request after logout returned %d: %s", response.Code, response.Body.String())
	}
}

func testHandler(db *sql.DB) http.Handler {
	return New(config.Config{PublicURL: "http://localhost"}, db, updater.New(updater.Options{Current: "dev"}), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func serveJSON(handler http.Handler, method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
