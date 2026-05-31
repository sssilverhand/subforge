-- +migrate Up

-- ─── Bot users ───────────────────────────────────────────────────────────────
-- Every Telegram user who has ever /started the bot.

CREATE TABLE bot_users (
    chat_id    BIGINT      PRIMARY KEY,
    username   TEXT,
    first_name TEXT,
    last_name  TEXT,
    joined_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE subscriptions
    ADD COLUMN telegram_chat_id BIGINT UNIQUE REFERENCES bot_users(chat_id) ON DELETE SET NULL;

ALTER TABLE admin_users
    ADD COLUMN telegram_chat_id BIGINT UNIQUE;

-- ─── Node agent ──────────────────────────────────────────────────────────────
-- Agent runs on each VPS and manages xray/hysteria2 binaries + systemd.

ALTER TABLE nodes
    ADD COLUMN agent_url     TEXT,         -- http://host:9090
    ADD COLUMN agent_secret  TEXT,         -- bearer token for agent auth
    ADD COLUMN agent_version TEXT,         -- last reported agent version
    ADD COLUMN agent_last_seen TIMESTAMPTZ;
