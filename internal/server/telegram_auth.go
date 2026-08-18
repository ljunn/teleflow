package server

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	tgauth "github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	"github.com/ljunn/teleflow/internal/config"
)

var errTelegramNotConfigured = errors.New("telegram API is not configured")

type accountAuthResult struct {
	Status      string
	CodeHash    string
	UserID      int64
	Username    string
	DisplayName string
}

type accountAuthorizer interface {
	RequestCode(ctx context.Context, accountID int64, phone string) (accountAuthResult, error)
	VerifyCode(ctx context.Context, accountID int64, phone, codeHash, code string) (accountAuthResult, error)
	VerifyPassword(ctx context.Context, accountID int64, password string) (accountAuthResult, error)
}

type gotdAccountAuthorizer struct {
	cfg   config.Config
	db    *sql.DB
	locks sync.Map
}

func newGotdAccountAuthorizer(cfg config.Config, db *sql.DB) accountAuthorizer {
	return &gotdAccountAuthorizer{cfg: cfg, db: db}
}

func (a *gotdAccountAuthorizer) RequestCode(ctx context.Context, accountID int64, phone string) (accountAuthResult, error) {
	var result accountAuthResult
	err := a.run(ctx, accountID, func(runCtx context.Context, client *telegram.Client) error {
		sent, err := client.Auth().SendCode(runCtx, phone, tgauth.SendCodeOptions{})
		if err != nil {
			return err
		}
		switch value := sent.(type) {
		case *tg.AuthSentCode:
			result = accountAuthResult{Status: "code_sent", CodeHash: value.PhoneCodeHash}
			return nil
		case *tg.AuthSentCodeSuccess:
			authorization, ok := value.Authorization.(*tg.AuthAuthorization)
			if !ok {
				return fmt.Errorf("unexpected authorization response %T", value.Authorization)
			}
			result = resultFromAuthorization(authorization)
			return nil
		default:
			return fmt.Errorf("unexpected send-code response %T", sent)
		}
	})
	return result, err
}

func (a *gotdAccountAuthorizer) VerifyCode(ctx context.Context, accountID int64, phone, codeHash, code string) (accountAuthResult, error) {
	var result accountAuthResult
	err := a.run(ctx, accountID, func(runCtx context.Context, client *telegram.Client) error {
		authorization, err := client.Auth().SignIn(runCtx, phone, code, codeHash)
		if errors.Is(err, tgauth.ErrPasswordAuthNeeded) {
			result = accountAuthResult{Status: "password_required"}
			return nil
		}
		if err != nil {
			return err
		}
		result = resultFromAuthorization(authorization)
		return nil
	})
	return result, err
}

func (a *gotdAccountAuthorizer) VerifyPassword(ctx context.Context, accountID int64, password string) (accountAuthResult, error) {
	var result accountAuthResult
	err := a.run(ctx, accountID, func(runCtx context.Context, client *telegram.Client) error {
		authorization, err := client.Auth().Password(runCtx, password)
		if err != nil {
			return err
		}
		result = resultFromAuthorization(authorization)
		return nil
	})
	return result, err
}

func (a *gotdAccountAuthorizer) run(ctx context.Context, accountID int64, callback func(context.Context, *telegram.Client) error) error {
	if a.cfg.TelegramAPIID <= 0 || a.cfg.TelegramAPIHash == "" {
		return errTelegramNotConfigured
	}
	if len(a.cfg.SessionKey) != 32 {
		return errors.New("session encryption key is unavailable")
	}

	lockValue, _ := a.locks.LoadOrStore(accountID, &sync.Mutex{})
	lock := lockValue.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()

	runCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	client := telegram.NewClient(a.cfg.TelegramAPIID, a.cfg.TelegramAPIHash, telegram.Options{
		SessionStorage: &encryptedSessionStorage{db: a.db, accountID: accountID, key: a.cfg.SessionKey},
		NoUpdates:      true,
	})
	return client.Run(runCtx, func(clientCtx context.Context) error {
		return callback(clientCtx, client)
	})
}

func resultFromAuthorization(authorization *tg.AuthAuthorization) accountAuthResult {
	result := accountAuthResult{Status: "authorized"}
	user, ok := authorization.User.(*tg.User)
	if !ok {
		return result
	}
	result.UserID = user.ID
	result.Username = user.Username
	result.DisplayName = strings.TrimSpace(strings.Join([]string{user.FirstName, user.LastName}, " "))
	return result
}

func telegramErrorMessage(err error) string {
	switch {
	case errors.Is(err, errTelegramNotConfigured):
		return "请先配置 Telegram API ID 和 API Hash"
	case errors.Is(err, tgauth.ErrPasswordInvalid):
		return "两步验证密码不正确"
	case tgerr.Is(err, "PHONE_CODE_INVALID"):
		return "验证码不正确"
	case tgerr.Is(err, "PHONE_CODE_EXPIRED"):
		return "验证码已过期，请重新发送"
	case tgerr.Is(err, "PHONE_NUMBER_INVALID"):
		return "Telegram 不接受该手机号"
	case tgerr.Is(err, "PHONE_NUMBER_BANNED"):
		return "该 Telegram 账号已被限制"
	case tgerr.Is(err, "FLOOD_WAIT"):
		return "Telegram 请求过于频繁，请稍后再试"
	case errors.Is(err, context.DeadlineExceeded):
		return "连接 Telegram 超时，请稍后再试"
	default:
		return "Telegram 授权失败，请稍后重试"
	}
}

type encryptedSessionStorage struct {
	db        *sql.DB
	accountID int64
	key       []byte
}

func (s *encryptedSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	var encrypted []byte
	if err := s.db.QueryRowContext(ctx, "SELECT session_ciphertext FROM telegram_accounts WHERE id = ?", s.accountID).Scan(&encrypted); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, session.ErrNotFound
		}
		return nil, fmt.Errorf("load encrypted session: %w", err)
	}
	if len(encrypted) == 0 {
		return nil, session.ErrNotFound
	}
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return nil, fmt.Errorf("create session cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create session AEAD: %w", err)
	}
	if len(encrypted) < gcm.NonceSize() {
		return nil, errors.New("encrypted session is truncated")
	}
	nonce, ciphertext := encrypted[:gcm.NonceSize()], encrypted[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertext, []byte(strconv.FormatInt(s.accountID, 10)))
	if err != nil {
		return nil, errors.New("decrypt Telegram session")
	}
	return plain, nil
}

func (s *encryptedSessionStorage) StoreSession(ctx context.Context, data []byte) error {
	block, err := aes.NewCipher(s.key)
	if err != nil {
		return fmt.Errorf("create session cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("create session AEAD: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("generate session nonce: %w", err)
	}
	encrypted := gcm.Seal(nonce, nonce, data, []byte(strconv.FormatInt(s.accountID, 10)))
	result, err := s.db.ExecContext(ctx, "UPDATE telegram_accounts SET session_ciphertext = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", encrypted, s.accountID)
	if err != nil {
		return fmt.Errorf("store encrypted session: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
