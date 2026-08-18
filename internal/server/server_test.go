package server

import (
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

func TestHealthAndSystemInfo(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	handler := New(config.Config{PublicURL: "http://localhost"}, db, updater.New(updater.Options{Current: "dev"}), slog.New(slog.NewTextHandler(io.Discard, nil)))

	for _, path := range []string{"/health/live", "/health/ready", "/api/v1/system/info", "/api/v1/overview"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<html") {
		t.Fatalf("frontend returned %d: %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/system/update", nil)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("update without repository returned %d: %s", response.Code, response.Body.String())
	}
}
