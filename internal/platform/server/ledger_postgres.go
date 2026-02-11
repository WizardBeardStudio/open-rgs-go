package server

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func (s *LedgerService) dbEnabled() bool {
	return s != nil && s.db != nil
}

func (s *LedgerService) ensureLedgerAccountTx(ctx context.Context, tx *sql.Tx, accountID, currency string) error {
	accountType := "player_cashless"
	playerID := accountID
	if accountID == "operator_liability" {
		accountType = "operator_liability"
		playerID = ""
	}
	if strings.HasPrefix(accountID, "device_escrow") {
		accountType = "device_escrow"
		playerID = ""
	}
	const q = `
INSERT INTO ledger_accounts (account_id, player_id, account_type, status, currency_code)
VALUES ($1, NULLIF($2,''), $3::ledger_account_type, 'active'::ledger_account_status, $4)
ON CONFLICT (account_id) DO NOTHING
`
	_, err := tx.ExecContext(ctx, q, accountID, playerID, accountType, strings.ToUpper(currency))
	return err
}

func (s *LedgerService) persistLedgerMutation(ctx context.Context, txRecord *rgsv1.LedgerTransaction, postings []ledgerPosting, status string, idemKey string) error {
	if !s.dbEnabled() || txRecord == nil {
		return nil
	}
	if len(postings) == 0 {
		return nil
	}

	dbtx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = dbtx.Rollback()
	}()

	for _, p := range postings {
		if err := s.ensureLedgerAccountTx(ctx, dbtx, p.accountID, p.currency); err != nil {
			return err
		}
	}

	const insTx = `
INSERT INTO ledger_transactions (
  transaction_id, request_id, idempotency_key, account_id, transaction_type, status,
  amount_minor, currency_code, authorization_id, denial_reason,
  actor_id, actor_type, source_device_id, occurred_at, received_at, recorded_at
)
VALUES ($1,$2,$3,$4,$5::ledger_transaction_type,$6::ledger_transaction_status,$7,$8,$9,$10,$11,$12,$13,$14::timestamptz,$15::timestamptz,NOW())
ON CONFLICT (transaction_id) DO NOTHING
`
	occurred := txRecord.OccurredAt
	if occurred == "" {
		occurred = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err = dbtx.ExecContext(ctx, insTx,
		txRecord.TransactionId,
		"", // request_id currently not materialized per-op
		idemKey,
		txRecord.AccountId,
		ledgerTxTypeToDB(txRecord.TransactionType),
		status,
		txRecord.Amount.GetAmountMinor(),
		strings.ToUpper(txRecord.Amount.GetCurrency()),
		txRecord.AuthorizationId,
		"",
		"",
		"",
		"",
		occurred,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return err
	}

	const insPosting = `
INSERT INTO ledger_postings (transaction_id, account_id, direction, amount_minor, currency_code)
VALUES ($1,$2,$3::ledger_posting_direction,$4,$5)
`
	for _, p := range postings {
		_, err := dbtx.ExecContext(ctx, insPosting,
			txRecord.TransactionId,
			p.accountID,
			p.direction,
			p.amount,
			strings.ToUpper(p.currency),
		)
		if err != nil {
			return err
		}
	}

	const adjust = `
UPDATE ledger_accounts
SET available_balance_minor = available_balance_minor + $2,
    updated_at = NOW()
WHERE account_id = $1
`
	for _, p := range postings {
		delta := p.amount
		if p.direction == "debit" {
			delta = -p.amount
		}
		if _, err := dbtx.ExecContext(ctx, adjust, p.accountID, delta); err != nil {
			return err
		}
	}

	if err := dbtx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *LedgerService) getBalanceFromDB(ctx context.Context, accountID string) (int64, int64, string, bool, error) {
	if !s.dbEnabled() {
		return 0, 0, "", false, nil
	}
	const q = `
SELECT available_balance_minor, pending_balance_minor, currency_code
FROM ledger_accounts
WHERE account_id = $1
`
	var available, pending int64
	var currency string
	err := s.db.QueryRowContext(ctx, q, accountID).Scan(&available, &pending, &currency)
	if err == sql.ErrNoRows {
		return 0, 0, "", false, nil
	}
	if err != nil {
		return 0, 0, "", false, err
	}
	return available, pending, currency, true, nil
}

func (s *LedgerService) listTransactionsFromDB(ctx context.Context, accountID string, limit, offset int) ([]*rgsv1.LedgerTransaction, error) {
	if !s.dbEnabled() {
		return nil, nil
	}
	const q = `
SELECT transaction_id, account_id, transaction_type::text, amount_minor, currency_code, occurred_at, authorization_id
FROM ledger_transactions
WHERE account_id = $1
ORDER BY recorded_at DESC
LIMIT $2 OFFSET $3
`
	rows, err := s.db.QueryContext(ctx, q, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.LedgerTransaction, 0)
	for rows.Next() {
		var txID, acctID, typ, currency, occurred, authID string
		var amount int64
		if err := rows.Scan(&txID, &acctID, &typ, &amount, &currency, &occurred, &authID); err != nil {
			return nil, err
		}
		out = append(out, &rgsv1.LedgerTransaction{
			TransactionId:   txID,
			AccountId:       acctID,
			TransactionType: ledgerTxTypeFromDB(typ),
			Amount:          money(amount, currency),
			OccurredAt:      occurred,
			AuthorizationId: authID,
		})
	}
	return out, rows.Err()
}

func (s *LedgerService) findTransactionByIdempotency(ctx context.Context, accountID string, txType rgsv1.LedgerTransactionType, idemKey string) (*rgsv1.LedgerTransaction, bool, error) {
	if !s.dbEnabled() {
		return nil, false, nil
	}
	const q = `
SELECT transaction_id, account_id, transaction_type::text, amount_minor, currency_code, occurred_at, authorization_id
FROM ledger_transactions
WHERE account_id = $1
  AND transaction_type = $2::ledger_transaction_type
  AND idempotency_key = $3
ORDER BY recorded_at DESC
LIMIT 1
`
	var txID, acctID, typ, currency, authID string
	var amount int64
	var occurred time.Time
	err := s.db.QueryRowContext(ctx, q, accountID, ledgerTxTypeToDB(txType), idemKey).Scan(
		&txID, &acctID, &typ, &amount, &currency, &occurred, &authID,
	)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &rgsv1.LedgerTransaction{
		TransactionId:   txID,
		AccountId:       acctID,
		TransactionType: ledgerTxTypeFromDB(typ),
		Amount:          money(amount, currency),
		OccurredAt:      occurred.UTC().Format(time.RFC3339Nano),
		AuthorizationId: authID,
	}, true, nil
}

func ledgerTxTypeToDB(v rgsv1.LedgerTransactionType) string {
	switch v {
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_DEPOSIT:
		return "deposit"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_WITHDRAWAL:
		return "withdrawal"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_DEVICE:
		return "transfer_to_device"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_ACCOUNT:
		return "transfer_to_account"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_GAMEPLAY_DEBIT:
		return "gameplay_debit"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_GAMEPLAY_CREDIT:
		return "gameplay_credit"
	case rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_MANUAL_ADJUSTMENT:
		return "manual_adjustment"
	default:
		return "manual_adjustment"
	}
}

func ledgerTxTypeFromDB(v string) rgsv1.LedgerTransactionType {
	switch v {
	case "deposit":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_DEPOSIT
	case "withdrawal":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_WITHDRAWAL
	case "transfer_to_device":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_DEVICE
	case "transfer_to_account":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_TRANSFER_TO_ACCOUNT
	case "gameplay_debit":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_GAMEPLAY_DEBIT
	case "gameplay_credit":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_GAMEPLAY_CREDIT
	case "manual_adjustment":
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_MANUAL_ADJUSTMENT
	default:
		return rgsv1.LedgerTransactionType_LEDGER_TRANSACTION_TYPE_UNSPECIFIED
	}
}

func must(err error) {
	if err != nil {
		panic(fmt.Sprintf("ledger postgres internal error: %v", err))
	}
}
