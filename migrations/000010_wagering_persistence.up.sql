CREATE TABLE IF NOT EXISTS wagers (
    wager_id TEXT PRIMARY KEY,
    player_id TEXT NOT NULL,
    game_id TEXT NOT NULL,
    stake_amount_minor BIGINT NOT NULL CHECK (stake_amount_minor > 0),
    stake_currency CHAR(3) NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'settled', 'canceled')),
    payout_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (payout_amount_minor >= 0),
    payout_currency CHAR(3) NOT NULL DEFAULT '',
    outcome_ref TEXT NOT NULL DEFAULT '',
    placed_at TIMESTAMPTZ NOT NULL,
    settled_at TIMESTAMPTZ,
    canceled_at TIMESTAMPTZ,
    cancel_reason TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wagers_player_time
    ON wagers(player_id, placed_at DESC);

CREATE INDEX IF NOT EXISTS idx_wagers_status_time
    ON wagers(status, recorded_at DESC);

CREATE TABLE IF NOT EXISTS wagering_idempotency_keys (
    operation TEXT NOT NULL,
    scope_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    response_payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (operation, scope_id, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_wagering_idempotency_expires
    ON wagering_idempotency_keys(expires_at);
