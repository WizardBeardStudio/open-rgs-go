package server

import (
	"context"
	"database/sql"
	"strings"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
)

func (s *EventsService) ensureEquipmentRowTx(ctx context.Context, tx *sql.Tx, equipmentID string) error {
	if strings.TrimSpace(equipmentID) == "" {
		return nil
	}
	const q = `
INSERT INTO equipment_registry (equipment_id, status)
VALUES ($1, 'active'::equipment_status)
ON CONFLICT (equipment_id) DO NOTHING
`
	_, err := tx.ExecContext(ctx, q, equipmentID)
	return err
}

func (s *EventsService) persistSignificantEvent(ctx context.Context, meta *rgsv1.RequestMeta, e *rgsv1.SignificantEvent, buffer ingestionBufferRecord) error {
	if s == nil || s.db == nil || e == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.ensureEquipmentRowTx(ctx, tx, e.EquipmentId); err != nil {
		return err
	}

	const insEvent = `
INSERT INTO significant_events (
  event_id, equipment_id, event_code, localized_description, severity,
  occurred_at, received_at, recorded_at, source_event_id, request_id,
  actor_id, actor_type, tags, payload
) VALUES (
  $1,$2,$3,$4,$5,$6::timestamptz,$7::timestamptz,$8::timestamptz,$9,$10,$11,$12,$13::jsonb,$14::jsonb
)
ON CONFLICT (event_id) DO NOTHING
`
	requestID := requestID(meta)
	actorID, actorType := "", ""
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	_, err = tx.ExecContext(ctx, insEvent,
		e.EventId,
		e.EquipmentId,
		e.EventCode,
		e.LocalizedDescription,
		e.Severity.String(),
		nonEmptyTS(e.OccurredAt),
		nonEmptyTS(e.ReceivedAt),
		nonEmptyTS(e.RecordedAt),
		e.EventId,
		requestID,
		actorID,
		actorType,
		`{}`,
		`{}`,
	)
	if err != nil {
		return err
	}

	if err := s.persistBufferTx(ctx, tx, "significant_event", buffer, requestID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *EventsService) persistMeterRecord(ctx context.Context, meta *rgsv1.RequestMeta, m *rgsv1.MeterRecord, buffer ingestionBufferRecord) error {
	if s == nil || s.db == nil || m == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := s.ensureEquipmentRowTx(ctx, tx, m.EquipmentId); err != nil {
		return err
	}

	const insMeter = `
INSERT INTO meter_records (
  meter_id, equipment_id, meter_label, monetary_unit, record_kind,
  value_minor, delta_minor, occurred_at, received_at, recorded_at,
  source_meter_id, request_id, actor_id, actor_type, tags, payload
) VALUES (
  $1,$2,$3,$4,$5::ingestion_record_kind,$6,$7,$8::timestamptz,$9::timestamptz,$10::timestamptz,$11,$12,$13,$14,$15::jsonb,$16::jsonb
)
ON CONFLICT (meter_id) DO NOTHING
`
	requestID := requestID(meta)
	actorID, actorType := "", ""
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	_, err = tx.ExecContext(ctx, insMeter,
		m.MeterId,
		m.EquipmentId,
		m.MeterLabel,
		strings.ToUpper(m.MonetaryUnit),
		meterKindToDB(m.RecordType),
		m.ValueMinor,
		m.DeltaMinor,
		nonEmptyTS(m.OccurredAt),
		nonEmptyTS(m.ReceivedAt),
		nonEmptyTS(m.RecordedAt),
		m.MeterId,
		requestID,
		actorID,
		actorType,
		`{}`,
		`{}`,
	)
	if err != nil {
		return err
	}

	if err := s.persistBufferTx(ctx, tx, "meter_snapshot", buffer, requestID); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *EventsService) persistBufferTx(ctx context.Context, tx *sql.Tx, kind string, buffer ingestionBufferRecord, requestID string) error {
	const insBuffer = `
INSERT INTO ingestion_buffers (
  record_kind, status, equipment_id, source_record_id, request_id,
  occurred_at, received_at, queued_at, payload
) VALUES (
  $1::ingestion_record_kind, $2::ingestion_buffer_status, $3, $4, $5,
  $6::timestamptz, $7::timestamptz, NOW(), $8::jsonb
)
`
	_, err := tx.ExecContext(ctx, insBuffer,
		kind,
		"acknowledged",
		buffer.equipmentID,
		buffer.sourceRecordID,
		requestID,
		nonEmptyTS(buffer.occurredAt),
		nonEmptyTS(buffer.receivedAt),
		`{}`,
	)
	return err
}

func (s *EventsService) listEventsFromDB(ctx context.Context, equipmentID string, limit, offset int) ([]*rgsv1.SignificantEvent, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT event_id, equipment_id, event_code, localized_description, severity,
       occurred_at, received_at, recorded_at
FROM significant_events
WHERE ($1 = '' OR equipment_id = $1)
ORDER BY recorded_at ASC, event_id ASC
LIMIT $2 OFFSET $3
`
	rows, err := s.db.QueryContext(ctx, q, equipmentID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.SignificantEvent, 0)
	for rows.Next() {
		var eventID, eqID, code, desc, sev string
		var occurred, received, recorded time.Time
		if err := rows.Scan(&eventID, &eqID, &code, &desc, &sev, &occurred, &received, &recorded); err != nil {
			return nil, err
		}
		out = append(out, &rgsv1.SignificantEvent{
			EventId:              eventID,
			EquipmentId:          eqID,
			EventCode:            code,
			LocalizedDescription: desc,
			Severity:             eventSeverityFromDB(sev),
			OccurredAt:           occurred.UTC().Format(time.RFC3339Nano),
			ReceivedAt:           received.UTC().Format(time.RFC3339Nano),
			RecordedAt:           recorded.UTC().Format(time.RFC3339Nano),
		})
	}
	return out, rows.Err()
}

func (s *EventsService) listMetersFromDB(ctx context.Context, equipmentID, meterLabel string, limit, offset int) ([]*rgsv1.MeterRecord, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT meter_id, equipment_id, meter_label, monetary_unit, record_kind::text,
       value_minor, delta_minor, occurred_at, received_at, recorded_at
FROM meter_records
WHERE ($1 = '' OR equipment_id = $1)
  AND ($2 = '' OR meter_label = $2)
ORDER BY recorded_at ASC, meter_id ASC
LIMIT $3 OFFSET $4
`
	rows, err := s.db.QueryContext(ctx, q, equipmentID, meterLabel, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.MeterRecord, 0)
	for rows.Next() {
		var meterID, eqID, label, unit, kind string
		var valueMinor, deltaMinor int64
		var occurred, received, recorded time.Time
		if err := rows.Scan(&meterID, &eqID, &label, &unit, &kind, &valueMinor, &deltaMinor, &occurred, &received, &recorded); err != nil {
			return nil, err
		}
		out = append(out, &rgsv1.MeterRecord{
			MeterId:      meterID,
			EquipmentId:  eqID,
			MeterLabel:   label,
			MonetaryUnit: unit,
			RecordType:   meterKindFromDB(kind),
			ValueMinor:   valueMinor,
			DeltaMinor:   deltaMinor,
			OccurredAt:   occurred.UTC().Format(time.RFC3339Nano),
			ReceivedAt:   received.UTC().Format(time.RFC3339Nano),
			RecordedAt:   recorded.UTC().Format(time.RFC3339Nano),
		})
	}
	return out, rows.Err()
}

func meterKindToDB(v rgsv1.MeterRecordType) string {
	switch v {
	case rgsv1.MeterRecordType_METER_RECORD_TYPE_SNAPSHOT:
		return "meter_snapshot"
	case rgsv1.MeterRecordType_METER_RECORD_TYPE_DELTA:
		return "meter_delta"
	default:
		return "meter_snapshot"
	}
}

func meterKindFromDB(v string) rgsv1.MeterRecordType {
	switch v {
	case "meter_snapshot":
		return rgsv1.MeterRecordType_METER_RECORD_TYPE_SNAPSHOT
	case "meter_delta":
		return rgsv1.MeterRecordType_METER_RECORD_TYPE_DELTA
	default:
		return rgsv1.MeterRecordType_METER_RECORD_TYPE_UNSPECIFIED
	}
}

func eventSeverityFromDB(v string) rgsv1.EventSeverity {
	switch strings.ToUpper(v) {
	case "EVENT_SEVERITY_INFO", "INFO":
		return rgsv1.EventSeverity_EVENT_SEVERITY_INFO
	case "EVENT_SEVERITY_WARN", "WARN":
		return rgsv1.EventSeverity_EVENT_SEVERITY_WARN
	case "EVENT_SEVERITY_CRITICAL", "CRITICAL":
		return rgsv1.EventSeverity_EVENT_SEVERITY_CRITICAL
	default:
		return rgsv1.EventSeverity_EVENT_SEVERITY_UNSPECIFIED
	}
}

func nonEmptyTS(v string) string {
	if strings.TrimSpace(v) == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return v
}
