package server

import (
	"bytes"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ljunn/teleflow/internal/config"
	"github.com/ljunn/teleflow/internal/database"
)

func TestParseAccountImport(t *testing.T) {
	items, problems := parseAccountImport(strings.Join([]string{
		"# vendor batch",
		"+10000000000----https://example.com/code/primary/GetHTML",
		"https://example.com/code/secondary/GetHTML|8613800138000|获客账号 B",
		"not-an-account",
	}, "\n"))
	if len(items) != 2 || len(problems) != 1 {
		t.Fatalf("parsed %d items and %d errors", len(items), len(problems))
	}
	if items[0].Phone != "+10000000000" || !strings.HasSuffix(items[0].CodeURL, "/GetHTML") {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	if items[1].Phone != "+8613800138000" || items[1].DisplayName != "获客账号 B" {
		t.Fatalf("unexpected second item: %+v", items[1])
	}
	if problems[0].Line != 4 {
		t.Fatalf("unexpected problem: %+v", problems[0])
	}
}

func TestAccountSecretEncryption(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	secret := "https://example.com/private/GetHTML"
	encrypted, err := encryptAccountSecret(key, 7, "code-url", secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte(secret)) {
		t.Fatal("code URL was stored without encryption")
	}
	plain, err := decryptAccountSecret(key, 7, "code-url", encrypted)
	if err != nil || plain != secret {
		t.Fatalf("decrypted %q: %v", plain, err)
	}
	if _, err := decryptAccountSecret(key, 8, "code-url", encrypted); err == nil {
		t.Fatal("expected account-bound additional data to reject another account ID")
	}
}

func TestExtractTelegramCode(t *testing.T) {
	page := `<html><body><td>Telegram</td><td>Login code: 51234. Do not share it.</td></body></html>`
	if got := extractTelegramCode(page); got != "51234" {
		t.Fatalf("code = %q", got)
	}
	if got := extractTelegramCode(`{"sms":"你的 Telegram 验证码是 123456"}`); got != "123456" {
		t.Fatalf("JSON code = %q", got)
	}
	if got := extractTelegramCode("order 8829301122 at 2026"); got != "" {
		t.Fatalf("unexpected loose code %q", got)
	}
	credentials := extractTelegramPageCredentials(`<html><input value="unrelated"><input id="code" value="62519" readonly><input value="2026-08-19"><input id="pass2fa" value="secret42" readonly></html>`)
	if credentials.Code != "62519" || credentials.Password != "secret42" {
		t.Fatalf("unexpected structured credentials: code length %d, password length %d", len(credentials.Code), len(credentials.Password))
	}
}

func TestPublicAddressFilter(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "10.0.0.8", "169.254.169.254", "192.0.2.1", "::1", "2001:db8::1"} {
		if isPublicAddress(net.ParseIP(raw)) {
			t.Fatalf("expected %s to be blocked", raw)
		}
	}
	if !isPublicAddress(net.ParseIP("1.1.1.1")) {
		t.Fatal("expected public address to be allowed")
	}
}

func TestImportAccountsStoresEncryptedURLAndIsIdempotent(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "account-import.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	key := bytes.Repeat([]byte{0x31}, 32)
	g := gin.New()
	group := g.Group("/api/v1")
	registerOperationsRoutes(group, config.Config{SessionKey: key}, db, &fakeAccountAuthorizer{})

	body := `{"text":"+10000000000----https://example.com/private/GetHTML"}`
	first := serveJSON(g, http.MethodPost, "/api/v1/accounts/import", body, nil)
	assertResponse(t, first, http.StatusOK, `"added":1`)
	list := serveJSON(g, http.MethodGet, "/api/v1/accounts", "", nil)
	assertResponse(t, list, http.StatusOK, `"hasCodeUrl":true`)
	if strings.Contains(list.Body.String(), "GetHTML") {
		t.Fatal("account list exposed the code URL")
	}

	var encrypted []byte
	if err := db.QueryRow("SELECT code_url_ciphertext FROM telegram_accounts WHERE phone = ?", "+10000000000").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("example.com")) {
		t.Fatal("database contains a plaintext code URL")
	}

	second := serveJSON(g, http.MethodPost, "/api/v1/accounts/import", body, nil)
	assertResponse(t, second, http.StatusOK, `"skipped":1`)
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM telegram_accounts").Scan(&count); err != nil || count != 1 {
		t.Fatalf("account count = %d: %v", count, err)
	}
}
