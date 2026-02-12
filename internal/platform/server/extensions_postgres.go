package server

import (
	"context"
	"strconv"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func (s *PromotionsService) persistBonusTransaction(ctx context.Context, tx *rgsv1.BonusTransaction) error {
	if s == nil || s.db == nil || tx == nil {
		return nil
	}
	const q = `
INSERT INTO bonus_transactions (
  bonus_transaction_id, equipment_id, player_id, campaign_id, meter_name,
  amount_minor, currency_code, occurred_at, received_at, recorded_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8::timestamptz,NOW(),NOW())
ON CONFLICT (bonus_transaction_id) DO UPDATE SET
  equipment_id = EXCLUDED.equipment_id,
  player_id = EXCLUDED.player_id,
  campaign_id = EXCLUDED.campaign_id,
  meter_name = EXCLUDED.meter_name,
  amount_minor = EXCLUDED.amount_minor,
  currency_code = EXCLUDED.currency_code,
  occurred_at = EXCLUDED.occurred_at
`
	_, err := s.db.ExecContext(ctx, q,
		tx.BonusTransactionId,
		tx.EquipmentId,
		tx.PlayerId,
		tx.CampaignId,
		tx.MeterName,
		tx.Amount.GetAmountMinor(),
		tx.Amount.GetCurrency(),
		nonEmptyTime(tx.OccurredAt),
	)
	return err
}

func (s *PromotionsService) listBonusTransactionsFromDB(ctx context.Context, equipmentID string, limit int) ([]*rgsv1.BonusTransaction, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT bonus_transaction_id, equipment_id, player_id, campaign_id, meter_name,
       amount_minor, currency_code, occurred_at
FROM bonus_transactions
WHERE ($1 = '' OR equipment_id = $1)
ORDER BY occurred_at DESC, bonus_transaction_id DESC
LIMIT $2
`
	rows, err := s.db.QueryContext(ctx, q, equipmentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.BonusTransaction, 0, limit)
	for rows.Next() {
		var tx rgsv1.BonusTransaction
		var amount int64
		var currency string
		var occurredAt time.Time
		if err := rows.Scan(
			&tx.BonusTransactionId,
			&tx.EquipmentId,
			&tx.PlayerId,
			&tx.CampaignId,
			&tx.MeterName,
			&amount,
			&currency,
			&occurredAt,
		); err != nil {
			return nil, err
		}
		tx.Amount = &rgsv1.Money{AmountMinor: amount, Currency: currency}
		tx.OccurredAt = occurredAt.UTC().Format(time.RFC3339Nano)
		out = append(out, &tx)
	}
	return out, rows.Err()
}

func (s *PromotionsService) persistPromotionalAward(ctx context.Context, award *rgsv1.PromotionalAward) error {
	if s == nil || s.db == nil || award == nil {
		return nil
	}
	const q = `
INSERT INTO promotional_awards (
  promotional_award_id, player_id, award_type, campaign_id, amount_minor, currency_code, occurred_at, received_at, recorded_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7::timestamptz,NOW(),NOW())
ON CONFLICT (promotional_award_id) DO UPDATE SET
  player_id = EXCLUDED.player_id,
  award_type = EXCLUDED.award_type,
  campaign_id = EXCLUDED.campaign_id,
  amount_minor = EXCLUDED.amount_minor,
  currency_code = EXCLUDED.currency_code,
  occurred_at = EXCLUDED.occurred_at
`
	_, err := s.db.ExecContext(ctx, q,
		award.PromotionalAwardId,
		award.PlayerId,
		strconv.Itoa(int(award.AwardType)),
		award.CampaignId,
		award.Amount.GetAmountMinor(),
		award.Amount.GetCurrency(),
		nonEmptyTime(award.OccurredAt),
	)
	return err
}

func (s *PromotionsService) listPromotionalAwardsFromDB(ctx context.Context, playerID, campaignID string, limit, offset int) ([]*rgsv1.PromotionalAward, string, error) {
	if s == nil || s.db == nil {
		return nil, "", nil
	}
	const q = `
SELECT promotional_award_id, player_id, award_type, campaign_id, amount_minor, currency_code, occurred_at
FROM promotional_awards
WHERE ($1 = '' OR player_id = $1)
  AND ($2 = '' OR campaign_id = $2)
ORDER BY occurred_at DESC, promotional_award_id DESC
LIMIT $3 OFFSET $4
`
	rows, err := s.db.QueryContext(ctx, q, playerID, campaignID, limit, offset)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	out := make([]*rgsv1.PromotionalAward, 0, limit)
	for rows.Next() {
		var (
			awardTypeRaw string
			occurredAt   time.Time
			award        rgsv1.PromotionalAward
			amount       int64
			currency     string
		)
		if err := rows.Scan(
			&award.PromotionalAwardId,
			&award.PlayerId,
			&awardTypeRaw,
			&award.CampaignId,
			&amount,
			&currency,
			&occurredAt,
		); err != nil {
			return nil, "", err
		}
		awardTypeInt, _ := strconv.Atoi(awardTypeRaw)
		award.AwardType = rgsv1.PromotionalAwardType(awardTypeInt)
		award.Amount = &rgsv1.Money{AmountMinor: amount, Currency: currency}
		award.OccurredAt = occurredAt.UTC().Format(time.RFC3339Nano)
		out = append(out, &award)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = strconv.Itoa(offset + len(out))
	}
	return out, next, nil
}

func (s *UISystemOverlayService) persistSystemWindowEvent(ctx context.Context, ev *rgsv1.SystemWindowEvent) error {
	if s == nil || s.db == nil || ev == nil {
		return nil
	}
	const q = `
INSERT INTO system_window_events (
  event_id, equipment_id, player_id, window_id, event_type, details, event_time, received_at, recorded_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7::timestamptz,NOW(),NOW())
ON CONFLICT (event_id) DO UPDATE SET
  equipment_id = EXCLUDED.equipment_id,
  player_id = EXCLUDED.player_id,
  window_id = EXCLUDED.window_id,
  event_type = EXCLUDED.event_type,
  details = EXCLUDED.details,
  event_time = EXCLUDED.event_time
`
	_, err := s.db.ExecContext(ctx, q,
		ev.EventId,
		ev.EquipmentId,
		ev.PlayerId,
		ev.WindowId,
		strconv.Itoa(int(ev.EventType)),
		ev.Details,
		nonEmptyTime(ev.EventTime),
	)
	return err
}

func (s *UISystemOverlayService) listSystemWindowEventsFromDB(ctx context.Context, equipmentID string, fromTS, toTS time.Time, limit, offset int) ([]*rgsv1.SystemWindowEvent, string, error) {
	if s == nil || s.db == nil {
		return nil, "", nil
	}
	const q = `
SELECT event_id, equipment_id, player_id, window_id, event_type, details, event_time
FROM system_window_events
WHERE ($1 = '' OR equipment_id = $1)
  AND ($2::timestamptz IS NULL OR event_time >= $2::timestamptz)
  AND ($3::timestamptz IS NULL OR event_time <= $3::timestamptz)
ORDER BY event_time DESC, event_id DESC
LIMIT $4 OFFSET $5
`
	rows, err := s.db.QueryContext(ctx, q, equipmentID, nullTime(fromTS), nullTime(toTS), limit, offset)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	out := make([]*rgsv1.SystemWindowEvent, 0, limit)
	for rows.Next() {
		var (
			evTypeRaw string
			eventTime time.Time
			ev        rgsv1.SystemWindowEvent
		)
		if err := rows.Scan(
			&ev.EventId,
			&ev.EquipmentId,
			&ev.PlayerId,
			&ev.WindowId,
			&evTypeRaw,
			&ev.Details,
			&eventTime,
		); err != nil {
			return nil, "", err
		}
		evTypeInt, _ := strconv.Atoi(evTypeRaw)
		ev.EventType = rgsv1.SystemWindowEventType(evTypeInt)
		ev.EventTime = eventTime.UTC().Format(time.RFC3339Nano)
		out = append(out, &ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = strconv.Itoa(offset + len(out))
	}
	return out, next, nil
}
