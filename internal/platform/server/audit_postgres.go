package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/audit"
)

func normalizeAuditJSON(raw []byte) []byte {
	if len(raw) == 0 {
		return []byte(`{}`)
	}
	var tmp any
	if err := json.Unmarshal(raw, &tmp); err != nil {
		return []byte(`{}`)
	}
	return raw
}

func auditAuthContextJSON(v string) []byte {
	if v == "" {
		return []byte(`{}`)
	}
	b, err := json.Marshal(map[string]string{"context": v})
	if err != nil {
		return []byte(`{}`)
	}
	return b
}

func appendAuditEventToDB(ctx context.Context, db *sql.DB, ev audit.Event) error {
	if db == nil {
		return nil
	}
	if ev.RecordedAt.IsZero() {
		ev.RecordedAt = time.Now().UTC()
	}
	if ev.OccurredAt.IsZero() {
		ev.OccurredAt = ev.RecordedAt
	}
	if ev.PartitionDay == "" {
		ev.PartitionDay = ev.RecordedAt.UTC().Format("2006-01-02")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const lockQ = `
SELECT hash_curr
FROM audit_events
WHERE partition_day = $1::date
ORDER BY recorded_at DESC, audit_id DESC
LIMIT 1
FOR UPDATE
`
	prev := "GENESIS"
	if err := tx.QueryRowContext(ctx, lockQ, ev.PartitionDay).Scan(&prev); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	ev.HashPrev = prev
	ev.HashCurr = audit.ComputeHash(prev, ev)

	const insQ = `
INSERT INTO audit_events (
  audit_id, occurred_at, recorded_at,
  actor_id, actor_type, auth_context,
  object_type, object_id, action,
  before_state, after_state,
  result, reason,
  partition_day,
  hash_prev, hash_curr
)
VALUES (
  $1, $2::timestamptz, $3::timestamptz,
  $4, $5, $6::jsonb,
  $7, $8, $9,
  $10::jsonb, $11::jsonb,
  $12, $13,
  $14::date,
  $15, $16
)
ON CONFLICT (audit_id) DO NOTHING
`
	_, err = tx.ExecContext(ctx, insQ,
		ev.AuditID,
		ev.OccurredAt.UTC().Format(time.RFC3339Nano),
		ev.RecordedAt.UTC().Format(time.RFC3339Nano),
		ev.ActorID,
		ev.ActorType,
		auditAuthContextJSON(ev.AuthContext),
		ev.ObjectType,
		ev.ObjectID,
		ev.Action,
		normalizeAuditJSON(ev.Before),
		normalizeAuditJSON(ev.After),
		string(ev.Result),
		ev.Reason,
		ev.PartitionDay,
		ev.HashPrev,
		ev.HashCurr,
	)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func listAuditEventsFromDB(ctx context.Context, db *sql.DB, objectTypeFilter string, pageToken string, pageSize int32) ([]*rgsv1.AuditEventRecord, string, error) {
	if db == nil {
		return nil, "", nil
	}
	limit := int(pageSize)
	if limit <= 0 {
		limit = 100
	}
	start := 0
	if pageToken != "" {
		n, err := strconv.Atoi(pageToken)
		if err != nil || n < 0 {
			return nil, "", fmt.Errorf("invalid page token")
		}
		start = n
	}

	const q = `
SELECT audit_id, occurred_at, recorded_at, actor_id, actor_type, object_type, object_id, action, result, reason
FROM audit_events
WHERE ($1 = '' OR object_type = $1)
ORDER BY recorded_at DESC, audit_id DESC
LIMIT $2 OFFSET $3
`
	rows, err := db.QueryContext(ctx, q, objectTypeFilter, limit, start)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	out := make([]*rgsv1.AuditEventRecord, 0, limit)
	for rows.Next() {
		var (
			ev                     rgsv1.AuditEventRecord
			occurredAt, recordedAt time.Time
		)
		if err := rows.Scan(
			&ev.AuditId,
			&occurredAt,
			&recordedAt,
			&ev.ActorId,
			&ev.ActorType,
			&ev.ObjectType,
			&ev.ObjectId,
			&ev.Action,
			&ev.Result,
			&ev.Reason,
		); err != nil {
			return nil, "", err
		}
		ev.OccurredAt = occurredAt.UTC().Format(time.RFC3339Nano)
		ev.RecordedAt = recordedAt.UTC().Format(time.RFC3339Nano)
		out = append(out, &ev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) == limit {
		next = strconv.Itoa(start + len(out))
	}
	return out, next, nil
}

func verifyAuditChainFromDB(ctx context.Context, db *sql.DB, partitionDay string) error {
	if db == nil {
		return nil
	}
	const q = `
SELECT audit_id, occurred_at, recorded_at, actor_id, actor_type, object_type, object_id, action,
       before_state, after_state, result, reason, partition_day, hash_prev, hash_curr
FROM audit_events
WHERE ($1 = '' OR partition_day = $1::date)
ORDER BY partition_day ASC, recorded_at ASC, audit_id ASC
`
	rows, err := db.QueryContext(ctx, q, partitionDay)
	if err != nil {
		return err
	}
	defer rows.Close()

	lastByPartition := map[string]string{}
	for rows.Next() {
		var (
			ev                                  audit.Event
			beforeRaw, afterRaw                 []byte
			resultRaw, partitionRaw             string
			occurredAt, recordedAt, partitionTS time.Time
			storedPrev, storedCurr              string
		)
		if err := rows.Scan(
			&ev.AuditID,
			&occurredAt,
			&recordedAt,
			&ev.ActorID,
			&ev.ActorType,
			&ev.ObjectType,
			&ev.ObjectID,
			&ev.Action,
			&beforeRaw,
			&afterRaw,
			&resultRaw,
			&ev.Reason,
			&partitionTS,
			&storedPrev,
			&storedCurr,
		); err != nil {
			return err
		}
		partitionRaw = partitionTS.UTC().Format("2006-01-02")
		ev.PartitionDay = partitionRaw
		ev.OccurredAt = occurredAt.UTC()
		ev.RecordedAt = recordedAt.UTC()
		ev.Before = beforeRaw
		ev.After = afterRaw
		ev.Result = audit.Result(resultRaw)
		ev.HashPrev = storedPrev
		ev.HashCurr = storedCurr

		expectedPrev := lastByPartition[partitionRaw]
		if expectedPrev == "" {
			expectedPrev = "GENESIS"
		}
		if ev.HashPrev != expectedPrev {
			return fmt.Errorf("audit chain prev hash mismatch audit_id=%s expected=%s got=%s", ev.AuditID, expectedPrev, ev.HashPrev)
		}
		expectedCurr := audit.ComputeHash(expectedPrev, ev)
		if ev.HashCurr != expectedCurr {
			return fmt.Errorf("audit chain curr hash mismatch audit_id=%s", ev.AuditID)
		}
		lastByPartition[partitionRaw] = ev.HashCurr
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return nil
}
