CREATE TABLE IF NOT EXISTS identity_sessions (
    refresh_token TEXT PRIMARY KEY,
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_identity_sessions_expiry
    ON identity_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_identity_sessions_actor
    ON identity_sessions(actor_id, actor_type, expires_at DESC);
