package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func (s *WageringService) dbEnabled() bool {
	return s != nil && s.db != nil
}

func wageringStatusToDB(v rgsv1.WagerStatus) string {
	switch v {
	case rgsv1.WagerStatus_WAGER_STATUS_PENDING:
		return "pending"
	case rgsv1.WagerStatus_WAGER_STATUS_SETTLED:
		return "settled"
	case rgsv1.WagerStatus_WAGER_STATUS_CANCELED:
		return "canceled"
	default:
		return "pending"
	}
}

func wageringStatusFromDB(v string) rgsv1.WagerStatus {
	switch strings.ToLower(v) {
	case "pending":
		return rgsv1.WagerStatus_WAGER_STATUS_PENDING
	case "settled":
		return rgsv1.WagerStatus_WAGER_STATUS_SETTLED
	case "canceled":
		return rgsv1.WagerStatus_WAGER_STATUS_CANCELED
	default:
		return rgsv1.WagerStatus_WAGER_STATUS_UNSPECIFIED
	}
}

func hashWageringRequest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:])
}

func (s *WageringService) persistWager(ctx context.Context, w *rgsv1.Wager) error {
	if !s.dbEnabled() || w == nil {
		return nil
	}
	const q = `
INSERT INTO wagers (
  wager_id, player_id, game_id, stake_amount_minor, stake_currency, status,
  payout_amount_minor, payout_currency, outcome_ref, placed_at, settled_at, canceled_at, cancel_reason,
  occurred_at, received_at, recorded_at
)
VALUES (
  $1,$2,$3,$4,$5,$6,$7,$8,$9,$10::timestamptz,NULLIF($11,'')::timestamptz,NULLIF($12,'')::timestamptz,$13,
  $14::timestamptz,NOW(),NOW()
)
ON CONFLICT (wager_id) DO UPDATE SET
  player_id = EXCLUDED.player_id,
  game_id = EXCLUDED.game_id,
  stake_amount_minor = EXCLUDED.stake_amount_minor,
  stake_currency = EXCLUDED.stake_currency,
  status = EXCLUDED.status,
  payout_amount_minor = EXCLUDED.payout_amount_minor,
  payout_currency = EXCLUDED.payout_currency,
  outcome_ref = EXCLUDED.outcome_ref,
  placed_at = EXCLUDED.placed_at,
  settled_at = EXCLUDED.settled_at,
  canceled_at = EXCLUDED.canceled_at,
  cancel_reason = EXCLUDED.cancel_reason,
  occurred_at = EXCLUDED.occurred_at,
  received_at = NOW(),
  recorded_at = NOW()
`
	payoutAmount := int64(0)
	payoutCurrency := ""
	if w.Payout != nil {
		payoutAmount = w.Payout.AmountMinor
		payoutCurrency = w.Payout.Currency
	}
	occurred := w.PlacedAt
	if occurred == "" {
		occurred = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, q,
		w.WagerId,
		w.PlayerId,
		w.GameId,
		w.Stake.GetAmountMinor(),
		w.Stake.GetCurrency(),
		wageringStatusToDB(w.Status),
		payoutAmount,
		payoutCurrency,
		w.OutcomeRef,
		nonEmptyTime(w.PlacedAt),
		w.SettledAt,
		w.CanceledAt,
		w.CancelReason,
		occurred,
	)
	return err
}

func (s *WageringService) getWager(ctx context.Context, wagerID string) (*rgsv1.Wager, error) {
	if !s.dbEnabled() {
		return nil, nil
	}
	const q = `
SELECT wager_id, player_id, game_id, stake_amount_minor, stake_currency, status,
       payout_amount_minor, payout_currency, outcome_ref, placed_at, settled_at, canceled_at, cancel_reason
FROM wagers
WHERE wager_id = $1
`
	var (
		w                                                 rgsv1.Wager
		stakeAmount, payoutAmount                         int64
		stakeCurrency, status, payoutCurrency, outcomeRef string
		placedAt                                          time.Time
		settledAt, canceledAt                             sql.NullTime
		cancelReason                                      string
	)
	err := s.db.QueryRowContext(ctx, q, wagerID).Scan(
		&w.WagerId,
		&w.PlayerId,
		&w.GameId,
		&stakeAmount,
		&stakeCurrency,
		&status,
		&payoutAmount,
		&payoutCurrency,
		&outcomeRef,
		&placedAt,
		&settledAt,
		&canceledAt,
		&cancelReason,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	w.Stake = &rgsv1.Money{AmountMinor: stakeAmount, Currency: stakeCurrency}
	if payoutCurrency != "" || payoutAmount != 0 {
		w.Payout = &rgsv1.Money{AmountMinor: payoutAmount, Currency: payoutCurrency}
	}
	w.Status = wageringStatusFromDB(status)
	w.OutcomeRef = outcomeRef
	w.PlacedAt = placedAt.UTC().Format(time.RFC3339Nano)
	if settledAt.Valid {
		w.SettledAt = settledAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if canceledAt.Valid {
		w.CanceledAt = canceledAt.Time.UTC().Format(time.RFC3339Nano)
	}
	w.CancelReason = cancelReason
	return &w, nil
}

func (s *WageringService) loadIdempotencyResponse(ctx context.Context, operation, scopeID, idempotencyKey, requestHash string, out proto.Message) (bool, error) {
	if !s.dbEnabled() {
		return false, nil
	}
	const q = `
SELECT request_hash, response_payload
FROM wagering_idempotency_keys
WHERE operation = $1 AND scope_id = $2 AND idempotency_key = $3
`
	var storedHash string
	var payload []byte
	err := s.db.QueryRowContext(ctx, q, operation, scopeID, idempotencyKey).Scan(&storedHash, &payload)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if storedHash != requestHash {
		return false, errIdempotencyRequestMismatch
	}
	if err := protojson.Unmarshal(payload, out); err != nil {
		return false, err
	}
	return true, nil
}

func (s *WageringService) persistIdempotencyResponse(ctx context.Context, operation, scopeID, idempotencyKey, requestHash string, response proto.Message) error {
	if !s.dbEnabled() || response == nil {
		return nil
	}
	payload, err := protojson.Marshal(response)
	if err != nil {
		return err
	}
	const q = `
INSERT INTO wagering_idempotency_keys (
  operation, scope_id, idempotency_key, request_hash, response_payload, created_at, expires_at
)
VALUES ($1,$2,$3,$4,$5::jsonb,NOW(),NOW() + INTERVAL '24 hours')
ON CONFLICT (operation, scope_id, idempotency_key) DO UPDATE SET
  request_hash = EXCLUDED.request_hash,
  response_payload = EXCLUDED.response_payload,
  expires_at = EXCLUDED.expires_at
`
	_, err = s.db.ExecContext(ctx, q, operation, scopeID, idempotencyKey, requestHash, payload)
	return err
}
