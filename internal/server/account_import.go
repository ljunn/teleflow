package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

const maxCodePageBytes = 1 << 20

var (
	accountImportSeparator = regexp.MustCompile(`\s*(?:----|\\\||[|｜\t,;])\s*`)
	importPhonePattern     = regexp.MustCompile(`^\+?[0-9][0-9\s\-()]{5,}$`)
	htmlTagPattern         = regexp.MustCompile(`<[^>]+>`)
	keyedCodePattern       = regexp.MustCompile(`(?i)(?:login\s*code|code|验证码|登录码|コード)[^0-9]{0,20}([0-9]{5,6})`)
	looseCodePattern       = regexp.MustCompile(`(?:^|[^0-9])([0-9]{5,6})(?:[^0-9]|$)`)
	blockedAddressRanges   = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"),
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
)

type importedAccount struct {
	Phone       string
	CodeURL     string
	DisplayName string
}

type telegramPageCredentials struct {
	Code     string
	Password string
}

func parseAccountImport(text string) ([]importedAccount, []accountImportError) {
	items := make([]importedAccount, 0)
	problems := make([]accountImportError, 0)
	for index, raw := range strings.Split(text, "\n") {
		lineNumber := index + 1
		line := strings.TrimSpace(strings.TrimPrefix(raw, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		item, err := parseAccountImportLine(line)
		if err != nil {
			problems = append(problems, accountImportError{Line: lineNumber, Error: err.Error()})
			continue
		}
		items = append(items, item)
	}
	return items, problems
}

func parseAccountImportLine(line string) (importedAccount, error) {
	parts := accountImportSeparator.Split(line, -1)
	var item importedAccount
	extras := make([]string, 0, 1)
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		switch {
		case strings.HasPrefix(strings.ToLower(part), "https://") || strings.HasPrefix(strings.ToLower(part), "http://"):
			if item.CodeURL == "" {
				item.CodeURL = part
			}
		case importPhonePattern.MatchString(part):
			if item.Phone == "" {
				item.Phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(part)
			}
		default:
			extras = append(extras, part)
		}
	}
	if item.Phone == "" {
		return importedAccount{}, errors.New("无法识别手机号")
	}
	if !strings.HasPrefix(item.Phone, "+") {
		item.Phone = "+" + item.Phone
	}
	if !phonePattern.MatchString(item.Phone) {
		return importedAccount{}, errors.New("手机号格式不正确")
	}
	if item.CodeURL == "" {
		return importedAccount{}, errors.New("缺少取码链接")
	}
	parsedURL, err := validateCodeURL(item.CodeURL)
	if err != nil {
		return importedAccount{}, err
	}
	item.CodeURL = parsedURL.String()
	if len(extras) > 0 {
		item.DisplayName = extras[0]
		if len([]rune(item.DisplayName)) > 80 {
			return importedAccount{}, errors.New("账号名称不能超过 80 个字符")
		}
	}
	return item, nil
}

func validateCodeURL(raw string) (*url.URL, error) {
	if len(raw) > 2048 {
		return nil, errors.New("取码链接过长")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" {
		return nil, errors.New("取码链接必须是有效的公网 HTTPS 地址")
	}
	if parsed.User != nil {
		return nil, errors.New("取码链接不能包含 URL 用户凭据")
	}
	parsed.Fragment = ""
	return parsed, nil
}

func encryptAccountSecret(key []byte, accountID int64, purpose, value string) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("session encryption key is unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(value), accountSecretAAD(accountID, purpose)), nil
}

func decryptAccountSecret(key []byte, accountID int64, purpose string, encrypted []byte) (string, error) {
	if len(key) != 32 {
		return "", errors.New("session encryption key is unavailable")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(encrypted) < gcm.NonceSize() {
		return "", errors.New("encrypted account secret is truncated")
	}
	plain, err := gcm.Open(nil, encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():], accountSecretAAD(accountID, purpose))
	if err != nil {
		return "", errors.New("decrypt account secret")
	}
	return string(plain), nil
}

func accountSecretAAD(accountID int64, purpose string) []byte {
	return []byte(strconv.FormatInt(accountID, 10) + ":" + purpose)
}

func extractTelegramCode(raw string) string {
	if credentials := extractTelegramPageCredentials(raw); credentials.Code != "" {
		return credentials.Code
	}
	plain := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(stdhtml.UnescapeString(htmlTagPattern.ReplaceAllString(raw, " ")), " "))
	if match := keyedCodePattern.FindStringSubmatch(plain); len(match) > 1 {
		return match[1]
	}
	if match := looseCodePattern.FindStringSubmatch(plain); len(match) > 1 {
		return match[1]
	}
	return ""
}

func extractTelegramPageCredentials(raw string) telegramPageCredentials {
	document, err := xhtml.Parse(strings.NewReader(raw))
	if err != nil {
		return telegramPageCredentials{}
	}
	var credentials telegramPageCredentials
	var visit func(*xhtml.Node)
	visit = func(node *xhtml.Node) {
		if node.Type == xhtml.ElementNode && node.Data == "input" {
			var id, value string
			for _, attribute := range node.Attr {
				switch strings.ToLower(attribute.Key) {
				case "id":
					id = strings.ToLower(strings.TrimSpace(attribute.Val))
				case "value":
					value = strings.TrimSpace(attribute.Val)
				}
			}
			switch id {
			case "code":
				if matched, _ := regexp.MatchString(`^[0-9]{3,8}$`, value); matched {
					credentials.Code = value
				}
			case "pass2fa":
				if len(value) <= 256 {
					credentials.Password = value
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(document)
	return credentials
}

func fetchTelegramCode(ctx context.Context, rawURL string) (string, error) {
	credentials, err := fetchTelegramPageCredentials(ctx, rawURL)
	return credentials.Code, err
}

func fetchTelegramPageCredentials(ctx context.Context, rawURL string) (telegramPageCredentials, error) {
	parsed, err := validateCodeURL(rawURL)
	if err != nil {
		return telegramPageCredentials{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return telegramPageCredentials{}, err
	}
	request.Header.Set("Accept", "text/html,application/json,text/plain;q=0.9,*/*;q=0.5")
	request.Header.Set("User-Agent", "Teleflow/0.1 account-code-fetcher")
	response, err := publicHTTPSClient().Do(request)
	if err != nil {
		return telegramPageCredentials{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return telegramPageCredentials{}, fmt.Errorf("取码服务返回 HTTP %d", response.StatusCode)
	}
	contents, err := io.ReadAll(io.LimitReader(response.Body, maxCodePageBytes+1))
	if err != nil {
		return telegramPageCredentials{}, err
	}
	if len(contents) > maxCodePageBytes {
		return telegramPageCredentials{}, errors.New("取码页面超过 1 MiB")
	}
	credentials := extractTelegramPageCredentials(string(contents))
	if credentials.Code == "" {
		credentials.Code = extractTelegramCodeFromText(string(contents))
	}
	return credentials, nil
}

func extractTelegramCodeFromText(raw string) string {
	plain := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(stdhtml.UnescapeString(htmlTagPattern.ReplaceAllString(raw, " ")), " "))
	if match := keyedCodePattern.FindStringSubmatch(plain); len(match) > 1 {
		return match[1]
	}
	if match := looseCodePattern.FindStringSubmatch(plain); len(match) > 1 {
		return match[1]
	}
	return ""
}

func waitForTelegramCode(ctx context.Context, rawURL, baseline string) (telegramPageCredentials, error) {
	excluded := make(map[string]struct{}, 1)
	if baseline != "" {
		excluded[baseline] = struct{}{}
	}
	return waitForTelegramCredentials(ctx, rawURL, excluded)
}

func waitForTelegramCredentials(ctx context.Context, rawURL string, excluded map[string]struct{}) (telegramPageCredentials, error) {
	ticker := time.NewTicker(4 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		credentials, err := fetchTelegramPageCredentials(ctx, rawURL)
		if err == nil && credentials.Code != "" {
			if _, seen := excluded[credentials.Code]; !seen {
				return credentials, nil
			}
		}
		if err != nil {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return telegramPageCredentials{}, fmt.Errorf("等待新验证码超时: %w", lastErr)
			}
			return telegramPageCredentials{}, errors.New("等待新验证码超时")
		case <-ticker.C:
		}
	}
}

func publicHTTPSClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, candidate := range addresses {
				if isPublicAddress(candidate.IP) {
					return dialer.DialContext(ctx, network, net.JoinHostPort(candidate.IP.String(), port))
				}
			}
			return nil, errors.New("取码链接解析到了非公网地址")
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   20 * time.Second,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("取码链接重定向次数过多")
			}
			_, err := validateCodeURL(request.URL.String())
			return err
		},
	}
}

func isPublicAddress(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedAddressRanges {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
