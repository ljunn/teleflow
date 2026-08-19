-- +goose Up
ALTER TABLE telegram_accounts ADD COLUMN code_url_ciphertext BLOB;
ALTER TABLE telegram_accounts ADD COLUMN source_type TEXT NOT NULL DEFAULT 'manual';
ALTER TABLE telegram_accounts ADD COLUMN last_checked_at TEXT;

CREATE INDEX idx_telegram_accounts_last_checked ON telegram_accounts(last_checked_at DESC);

-- +goose Down
DROP INDEX idx_telegram_accounts_last_checked;
ALTER TABLE telegram_accounts DROP COLUMN last_checked_at;
ALTER TABLE telegram_accounts DROP COLUMN source_type;
ALTER TABLE telegram_accounts DROP COLUMN code_url_ciphertext;
