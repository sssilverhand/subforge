-- +migrate Up

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ─── Admin Users ────────────────────────────────────────────────────────────

CREATE TABLE admin_users (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    role          TEXT        NOT NULL DEFAULT 'operator',
    -- roles: super_admin | admin | operator
    is_active     BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- API tokens for bot / external integrations
CREATE TABLE api_tokens (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT        NOT NULL,
    token_hash  TEXT        NOT NULL UNIQUE,
    role        TEXT        NOT NULL DEFAULT 'operator',
    created_by  UUID        REFERENCES admin_users(id) ON DELETE SET NULL,
    expires_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Nodes ──────────────────────────────────────────────────────────────────

CREATE TABLE nodes (
    id              UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT    NOT NULL,
    -- connection to management APIs
    xray_api_addr   TEXT,           -- host:port for gRPC API (e.g. "127.0.0.1:10085")
    xray_api_tls    BOOLEAN NOT NULL DEFAULT FALSE,
    hy2_api_url     TEXT,           -- http://host:port
    hy2_api_secret  TEXT,           -- Authorization header value
    -- what clients connect to
    public_host     TEXT    NOT NULL,  -- public IP or domain
    is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ─── Inbounds ───────────────────────────────────────────────────────────────

-- One row per protocol per node.
-- protocol values: vless-xhttp | vless-reality | vless-ws | hysteria2
CREATE TABLE inbounds (
    id        UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    node_id   UUID    NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    tag       TEXT    NOT NULL,    -- xray inbound tag (e.g. "vless-xhttp-in") or "hy2"
    protocol  TEXT    NOT NULL,
    port      INT     NOT NULL,
    settings  JSONB   NOT NULL DEFAULT '{}',
    -- protocol-specific: SNI, path, publicKey, shortId, obfs, etc.
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (node_id, tag)
);

-- ─── Plans ──────────────────────────────────────────────────────────────────

CREATE TABLE plans (
    id                  UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    name                TEXT           NOT NULL,
    description         TEXT,
    price_usd           NUMERIC(10,2),              -- NULL = free / manual
    traffic_limit_bytes BIGINT,                     -- NULL = unlimited
    duration_days       INT,                        -- NULL = no expiry
    is_active           BOOLEAN        NOT NULL DEFAULT TRUE,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

-- ─── Subscriptions ──────────────────────────────────────────────────────────

CREATE TABLE subscriptions (
    id          UUID    PRIMARY KEY DEFAULT gen_random_uuid(),
    token       TEXT    NOT NULL UNIQUE,   -- random slug for /sub/{token}
    name        TEXT,                      -- human label ("John's phone")
    plan_id     UUID    REFERENCES plans(id) ON DELETE SET NULL,

    -- credentials used in every protocol
    uuid        UUID    NOT NULL DEFAULT gen_random_uuid(), -- VLESS user UUID
    hy2_password TEXT   NOT NULL,                           -- Hysteria2 password

    -- limits (override plan if set)
    traffic_limit_bytes BIGINT,     -- NULL = unlimited
    traffic_used_bytes  BIGINT      NOT NULL DEFAULT 0,
    expires_at          TIMESTAMPTZ,

    -- derived state (updated by scheduler)
    is_enabled          BOOLEAN NOT NULL DEFAULT TRUE,
    is_traffic_exceeded BOOLEAN NOT NULL DEFAULT FALSE,
    is_expired          BOOLEAN NOT NULL DEFAULT FALSE,

    created_by  UUID    REFERENCES admin_users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ
);

CREATE INDEX idx_subscriptions_token ON subscriptions(token);
CREATE INDEX idx_subscriptions_uuid  ON subscriptions(uuid);

-- Which inbounds are available for a subscription
CREATE TABLE subscription_inbounds (
    subscription_id UUID NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    inbound_id      UUID NOT NULL REFERENCES inbounds(id)      ON DELETE CASCADE,
    PRIMARY KEY (subscription_id, inbound_id)
);

-- ─── Traffic ────────────────────────────────────────────────────────────────

-- Periodic snapshots from xray stats API and hysteria2 API.
-- Deltas are summed into subscriptions.traffic_used_bytes by scheduler.
CREATE TABLE traffic_snapshots (
    id              BIGSERIAL   PRIMARY KEY,
    subscription_id UUID        NOT NULL REFERENCES subscriptions(id) ON DELETE CASCADE,
    inbound_id      UUID        NOT NULL REFERENCES inbounds(id)      ON DELETE CASCADE,
    bytes_up        BIGINT      NOT NULL DEFAULT 0,
    bytes_down      BIGINT      NOT NULL DEFAULT 0,
    snapshot_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_traffic_sub_time ON traffic_snapshots(subscription_id, snapshot_at DESC);

-- ─── Billing ────────────────────────────────────────────────────────────────

CREATE TABLE invoices (
    id                  UUID           PRIMARY KEY DEFAULT gen_random_uuid(),
    subscription_id     UUID           NOT NULL REFERENCES subscriptions(id),
    plan_id             UUID           REFERENCES plans(id),
    amount_usd          NUMERIC(10,2)  NOT NULL,
    currency            TEXT           NOT NULL DEFAULT 'USD',
    status              TEXT           NOT NULL DEFAULT 'pending',
    -- status: pending | paid | expired | cancelled
    payment_provider    TEXT,          -- cryptomus | stripe | telegram_stars
    external_id         TEXT,          -- provider's payment/invoice ID
    paid_at             TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ    NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_invoices_sub    ON invoices(subscription_id);
CREATE INDEX idx_invoices_ext_id ON invoices(payment_provider, external_id);
