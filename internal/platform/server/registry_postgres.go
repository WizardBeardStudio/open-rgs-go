package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
)

func (s *RegistryService) upsertEquipmentInDB(ctx context.Context, eq *rgsv1.Equipment) error {
	if s == nil || s.db == nil || eq == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	attrs, _ := json.Marshal(eq.Attributes)
	rtp, hasRTP := parseOptionalRTP(eq.TheoreticalRtpBps)

	const q = `
INSERT INTO equipment_registry (
  equipment_id, external_reference, location, status, theoretical_rtp_bps,
  control_program_version, config_version, attributes, created_at, updated_at
) VALUES (
  $1,$2,$3,$4::equipment_status,$5,$6,$7,$8::jsonb,$9::timestamptz,$10::timestamptz
)
ON CONFLICT (equipment_id) DO UPDATE SET
  external_reference = EXCLUDED.external_reference,
  location = EXCLUDED.location,
  status = EXCLUDED.status,
  theoretical_rtp_bps = EXCLUDED.theoretical_rtp_bps,
  control_program_version = EXCLUDED.control_program_version,
  config_version = EXCLUDED.config_version,
  attributes = EXCLUDED.attributes,
  updated_at = EXCLUDED.updated_at
`
	var rtpValue any
	if hasRTP {
		rtpValue = rtp
	}
	_, err = tx.ExecContext(ctx, q,
		eq.EquipmentId,
		eq.ExternalReference,
		eq.Location,
		equipmentStatusToDB(eq.Status),
		rtpValue,
		eq.ControlProgramVersion,
		eq.ConfigVersion,
		string(attrs),
		nonEmptyTimestamp(eq.CreatedAt),
		nonEmptyTimestamp(eq.UpdatedAt),
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *RegistryService) getEquipmentFromDB(ctx context.Context, equipmentID string) (*rgsv1.Equipment, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT equipment_id, external_reference, location, status::text, theoretical_rtp_bps,
       control_program_version, config_version, attributes, created_at, updated_at
FROM equipment_registry
WHERE equipment_id = $1
`
	var (
		id, extRef, location, status, controlProgramVersion, configVersion string
		attrJSON                                                           []byte
		rtp                                                                sql.NullInt32
		createdAt, updatedAt                                               time.Time
	)
	err := s.db.QueryRowContext(ctx, q, equipmentID).Scan(
		&id, &extRef, &location, &status, &rtp,
		&controlProgramVersion, &configVersion, &attrJSON, &createdAt, &updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	attrs := map[string]string{}
	if len(attrJSON) > 0 {
		_ = json.Unmarshal(attrJSON, &attrs)
	}
	eq := &rgsv1.Equipment{
		EquipmentId:           id,
		ExternalReference:     extRef,
		Location:              location,
		Status:                equipmentStatusFromDB(status),
		ControlProgramVersion: controlProgramVersion,
		ConfigVersion:         configVersion,
		CreatedAt:             createdAt.UTC().Format(time.RFC3339Nano),
		UpdatedAt:             updatedAt.UTC().Format(time.RFC3339Nano),
		Attributes:            attrs,
	}
	if rtp.Valid {
		eq.TheoreticalRtpBps = strconv.FormatInt(int64(rtp.Int32), 10)
	}
	return eq, nil
}

func (s *RegistryService) listEquipmentFromDB(ctx context.Context, filter rgsv1.EquipmentStatus, limit, offset int) ([]*rgsv1.Equipment, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	status := equipmentStatusToDB(filter)
	if filter == rgsv1.EquipmentStatus_EQUIPMENT_STATUS_UNSPECIFIED {
		status = ""
	}
	const q = `
SELECT equipment_id, external_reference, location, status::text, theoretical_rtp_bps,
       control_program_version, config_version, attributes, created_at, updated_at
FROM equipment_registry
WHERE ($1 = '' OR status::text = $1)
ORDER BY equipment_id ASC
LIMIT $2 OFFSET $3
`
	rows, err := s.db.QueryContext(ctx, q, status, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.Equipment, 0, limit)
	for rows.Next() {
		var (
			id, extRef, location, dbStatus, controlProgramVersion, configVersion string
			attrJSON                                                             []byte
			rtp                                                                  sql.NullInt32
			createdAt, updatedAt                                                 time.Time
		)
		if err := rows.Scan(
			&id, &extRef, &location, &dbStatus, &rtp,
			&controlProgramVersion, &configVersion, &attrJSON, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		attrs := map[string]string{}
		if len(attrJSON) > 0 {
			_ = json.Unmarshal(attrJSON, &attrs)
		}
		item := &rgsv1.Equipment{
			EquipmentId:           id,
			ExternalReference:     extRef,
			Location:              location,
			Status:                equipmentStatusFromDB(dbStatus),
			ControlProgramVersion: controlProgramVersion,
			ConfigVersion:         configVersion,
			CreatedAt:             createdAt.UTC().Format(time.RFC3339Nano),
			UpdatedAt:             updatedAt.UTC().Format(time.RFC3339Nano),
			Attributes:            attrs,
		}
		if rtp.Valid {
			item.TheoreticalRtpBps = strconv.FormatInt(int64(rtp.Int32), 10)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func equipmentStatusToDB(v rgsv1.EquipmentStatus) string {
	switch v {
	case rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE:
		return "active"
	case rgsv1.EquipmentStatus_EQUIPMENT_STATUS_INACTIVE:
		return "inactive"
	case rgsv1.EquipmentStatus_EQUIPMENT_STATUS_MAINTENANCE:
		return "maintenance"
	case rgsv1.EquipmentStatus_EQUIPMENT_STATUS_DISABLED:
		return "disabled"
	case rgsv1.EquipmentStatus_EQUIPMENT_STATUS_RETIRED:
		return "retired"
	default:
		return "active"
	}
}

func equipmentStatusFromDB(v string) rgsv1.EquipmentStatus {
	switch strings.ToLower(v) {
	case "active":
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_ACTIVE
	case "inactive":
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_INACTIVE
	case "maintenance":
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_MAINTENANCE
	case "disabled":
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_DISABLED
	case "retired":
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_RETIRED
	default:
		return rgsv1.EquipmentStatus_EQUIPMENT_STATUS_UNSPECIFIED
	}
}

func parseOptionalRTP(v string) (int32, bool) {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		return 0, false
	}
	return int32(parsed), true
}

func nonEmptyTimestamp(v string) string {
	if strings.TrimSpace(v) == "" {
		return time.Now().UTC().Format(time.RFC3339Nano)
	}
	return v
}
