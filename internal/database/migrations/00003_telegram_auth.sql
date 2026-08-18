-- +goose Up
ALTER TABLE telegram_accounts ADD COLUMN auth_code_hash TEXT NOT NULL DEFAULT '';
ALTER TABLE telegram_accounts ADD COLUMN auth_code_sent_at TEXT;
ALTER TABLE telegram_accounts ADD COLUMN telegram_user_id INTEGER;
ALTER TABLE telegram_accounts ADD COLUMN username TEXT NOT NULL DEFAULT '';
ALTER TABLE telegram_accounts ADD COLUMN last_error TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_telegram_accounts_status ON telegram_accounts(status, created_at DESC);

-- +goose Down
DROP INDEX idx_telegram_accounts_status;
ALTER TABLE telegram_accounts DROP COLUMN last_error;
ALTER TABLE telegram_accounts DROP COLUMN username;
ALTER TABLE telegram_accounts DROP COLUMN telegram_user_id;
ALTER TABLE telegram_accounts DROP COLUMN auth_code_sent_at;
ALTER TABLE telegram_accounts DROP COLUMN auth_code_hash;
