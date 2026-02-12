CREATE TABLE IF NOT EXISTS ledger_eft_lockouts (
  account_id TEXT PRIMARY KEY,
  failed_attempts INTEGER NOT NULL DEFAULT 0,
  locked_until TIMESTAMPTZ,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_eft_lockouts_locked_until
  ON ledger_eft_lockouts (locked_until DESC);
