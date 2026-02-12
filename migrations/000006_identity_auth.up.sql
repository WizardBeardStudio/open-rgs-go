CREATE TABLE IF NOT EXISTS identity_credentials (
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_id, actor_type)
);

CREATE TABLE IF NOT EXISTS identity_lockouts (
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    failed_attempts INTEGER NOT NULL DEFAULT 0,
    locked_until TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_id, actor_type)
);

CREATE INDEX IF NOT EXISTS idx_identity_lockouts_locked_until
    ON identity_lockouts(locked_until);
