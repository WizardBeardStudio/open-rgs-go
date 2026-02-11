CREATE TYPE ledger_account_type AS ENUM (
    'player_cashless',
    'operator_liability',
    'device_escrow',
    'system_settlement'
);

CREATE TYPE ledger_account_status AS ENUM (
    'active',
    'locked',
    'suspended',
    'closed'
);

CREATE TYPE ledger_transaction_type AS ENUM (
    'deposit',
    'withdrawal',
    'transfer_to_device',
    'transfer_to_account',
    'gameplay_debit',
    'gameplay_credit',
    'manual_adjustment'
);

CREATE TYPE ledger_transaction_status AS ENUM (
    'accepted',
    'denied',
    'pending',
    'unresolved',
    'reversed'
);

CREATE TYPE ledger_posting_direction AS ENUM (
    'debit',
    'credit'
);

CREATE TYPE unresolved_transfer_status AS ENUM (
    'open',
    'resolved',
    'cancelled'
);

CREATE TABLE IF NOT EXISTS ledger_accounts (
    account_id TEXT PRIMARY KEY,
    player_id TEXT,
    account_type ledger_account_type NOT NULL,
    status ledger_account_status NOT NULL DEFAULT 'active',
    currency_code CHAR(3) NOT NULL,
    available_balance_minor BIGINT NOT NULL DEFAULT 0 CHECK (available_balance_minor >= 0),
    pending_balance_minor BIGINT NOT NULL DEFAULT 0 CHECK (pending_balance_minor >= 0),
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    closed_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_accounts_player_currency_cashless
    ON ledger_accounts(player_id, currency_code)
    WHERE player_id IS NOT NULL AND account_type = 'player_cashless';

CREATE INDEX IF NOT EXISTS idx_ledger_accounts_player
    ON ledger_accounts(player_id)
    WHERE player_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS ledger_transactions (
    transaction_id TEXT PRIMARY KEY,
    request_id TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    account_id TEXT NOT NULL REFERENCES ledger_accounts(account_id),
    transaction_type ledger_transaction_type NOT NULL,
    status ledger_transaction_status NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency_code CHAR(3) NOT NULL,
    authorization_id TEXT NOT NULL DEFAULT '',
    denial_reason TEXT NOT NULL DEFAULT '',
    actor_id TEXT NOT NULL,
    actor_type TEXT NOT NULL,
    source_device_id TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    metadata JSONB NOT NULL DEFAULT '{}'::JSONB
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_ledger_transactions_idempotency
    ON ledger_transactions(account_id, transaction_type, idempotency_key);

CREATE INDEX IF NOT EXISTS idx_ledger_transactions_account_recorded
    ON ledger_transactions(account_id, recorded_at DESC);

CREATE TABLE IF NOT EXISTS ledger_postings (
    posting_id BIGSERIAL PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES ledger_transactions(transaction_id),
    account_id TEXT NOT NULL REFERENCES ledger_accounts(account_id),
    direction ledger_posting_direction NOT NULL,
    amount_minor BIGINT NOT NULL CHECK (amount_minor > 0),
    currency_code CHAR(3) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_postings_transaction
    ON ledger_postings(transaction_id);

CREATE TABLE IF NOT EXISTS ledger_idempotency_keys (
    scope TEXT NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash BYTEA NOT NULL,
    response_payload JSONB NOT NULL DEFAULT '{}'::JSONB,
    result_code TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (scope, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_ledger_idempotency_keys_expires
    ON ledger_idempotency_keys(expires_at);

CREATE TABLE IF NOT EXISTS cashless_unresolved_transfers (
    transfer_id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES ledger_accounts(account_id),
    device_id TEXT NOT NULL,
    requested_amount_minor BIGINT NOT NULL CHECK (requested_amount_minor > 0),
    transferred_amount_minor BIGINT NOT NULL DEFAULT 0 CHECK (transferred_amount_minor >= 0),
    currency_code CHAR(3) NOT NULL,
    status unresolved_transfer_status NOT NULL DEFAULT 'open',
    reason TEXT NOT NULL DEFAULT '',
    transaction_id TEXT REFERENCES ledger_transactions(transaction_id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CHECK (transferred_amount_minor <= requested_amount_minor)
);

CREATE INDEX IF NOT EXISTS idx_cashless_unresolved_transfers_status_created
    ON cashless_unresolved_transfers(status, created_at);

CREATE OR REPLACE FUNCTION set_row_updated_at()
RETURNS trigger AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_ledger_accounts_updated_at ON ledger_accounts;
CREATE TRIGGER tr_ledger_accounts_updated_at
BEFORE UPDATE ON ledger_accounts
FOR EACH ROW
EXECUTE FUNCTION set_row_updated_at();

CREATE OR REPLACE FUNCTION enforce_balanced_transaction()
RETURNS trigger AS $$
DECLARE
    tx_id TEXT;
    delta BIGINT;
BEGIN
    tx_id := COALESCE(NEW.transaction_id, OLD.transaction_id);

    SELECT COALESCE(
        SUM(
            CASE direction
                WHEN 'credit' THEN amount_minor
                WHEN 'debit' THEN -amount_minor
            END
        ),
        0
    )
    INTO delta
    FROM ledger_postings
    WHERE transaction_id = tx_id;

    IF delta <> 0 THEN
        RAISE EXCEPTION 'ledger transaction % is unbalanced (delta=%)', tx_id, delta;
    END IF;

    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS tr_ledger_postings_balanced ON ledger_postings;
CREATE CONSTRAINT TRIGGER tr_ledger_postings_balanced
AFTER INSERT OR UPDATE OR DELETE ON ledger_postings
DEFERRABLE INITIALLY DEFERRED
FOR EACH ROW
EXECUTE FUNCTION enforce_balanced_transaction();
