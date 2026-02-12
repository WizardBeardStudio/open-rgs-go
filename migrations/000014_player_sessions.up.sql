CREATE TABLE IF NOT EXISTS player_sessions (
    session_id TEXT PRIMARY KEY,
    player_id TEXT NOT NULL,
    device_id TEXT NOT NULL,
    state TEXT NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    ended_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    end_reason TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_player_sessions_player_state
    ON player_sessions(player_id, state, expires_at DESC);

CREATE INDEX IF NOT EXISTS idx_player_sessions_device_state
    ON player_sessions(device_id, state, expires_at DESC);
