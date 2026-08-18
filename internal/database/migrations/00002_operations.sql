-- +goose Up
CREATE TABLE discovery_tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    query TEXT NOT NULL,
    source_type TEXT NOT NULL DEFAULT 'public_chat',
    status TEXT NOT NULL DEFAULT 'pending_connection',
    result_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_discovery_tasks_status ON discovery_tasks(status, created_at DESC);

CREATE TABLE campaigns (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'direct_message',
    target TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    run_at TEXT,
    sent_count INTEGER NOT NULL DEFAULT 0,
    failed_count INTEGER NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_campaigns_status ON campaigns(status, created_at DESC);

CREATE TABLE relay_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    bot_username TEXT NOT NULL DEFAULT '',
    master_username TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO relay_settings(id) VALUES (1);

-- +goose Down
DROP TABLE relay_settings;
DROP TABLE campaigns;
DROP TABLE discovery_tasks;
