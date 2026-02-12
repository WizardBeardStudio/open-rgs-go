CREATE TABLE IF NOT EXISTS bonus_transactions (
    bonus_transaction_id TEXT PRIMARY KEY,
    equipment_id TEXT NOT NULL,
    player_id TEXT NOT NULL,
    campaign_id TEXT NOT NULL DEFAULT '',
    meter_name TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL DEFAULT 0,
    currency_code CHAR(3) NOT NULL DEFAULT 'USD',
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bonus_transactions_equipment_time
    ON bonus_transactions(equipment_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS promotional_awards (
    promotional_award_id TEXT PRIMARY KEY,
    player_id TEXT NOT NULL,
    award_type TEXT NOT NULL,
    campaign_id TEXT NOT NULL DEFAULT '',
    amount_minor BIGINT NOT NULL DEFAULT 0,
    currency_code CHAR(3) NOT NULL DEFAULT 'USD',
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_promotional_awards_player_time
    ON promotional_awards(player_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS system_window_events (
    event_id TEXT PRIMARY KEY,
    equipment_id TEXT NOT NULL,
    player_id TEXT NOT NULL DEFAULT '',
    window_id TEXT NOT NULL,
    event_type TEXT NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    event_time TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_window_events_equipment_time
    ON system_window_events(equipment_id, event_time DESC);
