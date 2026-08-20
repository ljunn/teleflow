-- +goose Up
ALTER TABLE telegram_accounts ADD COLUMN remark TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE telegram_accounts DROP COLUMN remark;
