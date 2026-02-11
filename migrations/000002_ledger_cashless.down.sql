DROP TRIGGER IF EXISTS tr_ledger_postings_balanced ON ledger_postings;
DROP FUNCTION IF EXISTS enforce_balanced_transaction();

DROP TRIGGER IF EXISTS tr_ledger_accounts_updated_at ON ledger_accounts;
DROP FUNCTION IF EXISTS set_row_updated_at();

DROP TABLE IF EXISTS cashless_unresolved_transfers;
DROP TABLE IF EXISTS ledger_idempotency_keys;
DROP TABLE IF EXISTS ledger_postings;
DROP TABLE IF EXISTS ledger_transactions;
DROP TABLE IF EXISTS ledger_accounts;

DROP TYPE IF EXISTS unresolved_transfer_status;
DROP TYPE IF EXISTS ledger_posting_direction;
DROP TYPE IF EXISTS ledger_transaction_status;
DROP TYPE IF EXISTS ledger_transaction_type;
DROP TYPE IF EXISTS ledger_account_status;
DROP TYPE IF EXISTS ledger_account_type;
