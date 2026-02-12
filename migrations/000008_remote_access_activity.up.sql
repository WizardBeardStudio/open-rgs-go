CREATE TABLE IF NOT EXISTS remote_access_activity (
    activity_id BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    source_ip TEXT NOT NULL,
    source_port TEXT NOT NULL DEFAULT '',
    destination_host TEXT NOT NULL,
    destination_port TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL,
    method TEXT NOT NULL,
    allowed BOOLEAN NOT NULL,
    reason TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_remote_access_activity_occurred
    ON remote_access_activity(occurred_at DESC);

CREATE INDEX IF NOT EXISTS idx_remote_access_activity_source
    ON remote_access_activity(source_ip, occurred_at DESC);
