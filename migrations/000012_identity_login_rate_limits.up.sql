CREATE TABLE IF NOT EXISTS identity_login_rate_limits (
  actor_id TEXT NOT NULL,
  actor_type TEXT NOT NULL,
  window_start TIMESTAMPTZ NOT NULL,
  attempt_count INTEGER NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (actor_id, actor_type)
);

CREATE INDEX IF NOT EXISTS idx_identity_login_rate_limits_window_start
  ON identity_login_rate_limits (window_start DESC);
