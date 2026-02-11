package server

import (
	"context"
	"database/sql"
	"strings"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func (s *ConfigService) persistConfigChange(ctx context.Context, c *rgsv1.ConfigChange) error {
	if s == nil || s.db == nil || c == nil {
		return nil
	}
	const q = `
INSERT INTO config_changes (
  change_id, config_namespace, config_key, proposed_value, previous_value, reason,
  status, proposer_id, approver_id, applied_by, created_at, approved_at, applied_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::timestamptz,NULLIF($12,'')::timestamptz,NULLIF($13,'')::timestamptz)
ON CONFLICT (change_id) DO UPDATE SET
  status = EXCLUDED.status,
  approver_id = EXCLUDED.approver_id,
  applied_by = EXCLUDED.applied_by,
  approved_at = EXCLUDED.approved_at,
  applied_at = EXCLUDED.applied_at,
  reason = EXCLUDED.reason
`
	_, err := s.db.ExecContext(ctx, q,
		c.ChangeId,
		c.ConfigNamespace,
		c.ConfigKey,
		c.ProposedValue,
		c.PreviousValue,
		c.Reason,
		configStatusToDB(c.Status),
		c.ProposerId,
		c.ApproverId,
		c.AppliedBy,
		nullIfEmpty(c.CreatedAt),
		nullIfEmpty(c.ApprovedAt),
		nullIfEmpty(c.AppliedAt),
	)
	return err
}

func (s *ConfigService) persistCurrentValue(ctx context.Context, namespace, key, value, updatedBy string) error {
	if s == nil || s.db == nil {
		return nil
	}
	const q = `
INSERT INTO config_current_values (config_namespace, config_key, value, updated_by)
VALUES ($1,$2,$3,$4)
ON CONFLICT (config_namespace, config_key) DO UPDATE SET
  value = EXCLUDED.value,
  updated_by = EXCLUDED.updated_by,
  updated_at = NOW()
`
	_, err := s.db.ExecContext(ctx, q, namespace, key, value, updatedBy)
	return err
}

func (s *ConfigService) persistDownloadEntry(ctx context.Context, e *rgsv1.DownloadLibraryEntry) error {
	if s == nil || s.db == nil || e == nil {
		return nil
	}
	const q = `
INSERT INTO download_library_changes (
  entry_id, library_path, checksum, version, action, changed_by, reason, occurred_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::timestamptz)
ON CONFLICT (entry_id) DO UPDATE SET
  checksum = EXCLUDED.checksum,
  version = EXCLUDED.version,
  action = EXCLUDED.action,
  changed_by = EXCLUDED.changed_by,
  reason = EXCLUDED.reason,
  occurred_at = EXCLUDED.occurred_at
`
	_, err := s.db.ExecContext(ctx, q,
		e.EntryId,
		e.LibraryPath,
		e.Checksum,
		e.Version,
		downloadActionToDB(e.Action),
		e.ChangedBy,
		e.Reason,
		nullIfEmpty(e.OccurredAt),
	)
	return err
}

func configStatusToDB(v rgsv1.ConfigChangeStatus) string {
	switch v {
	case rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_PROPOSED:
		return "proposed"
	case rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED:
		return "approved"
	case rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED:
		return "applied"
	case rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_REJECTED:
		return "rejected"
	default:
		return "proposed"
	}
}

func downloadActionToDB(v rgsv1.DownloadAction) string {
	switch v {
	case rgsv1.DownloadAction_DOWNLOAD_ACTION_ADD:
		return "add"
	case rgsv1.DownloadAction_DOWNLOAD_ACTION_UPDATE:
		return "update"
	case rgsv1.DownloadAction_DOWNLOAD_ACTION_DELETE:
		return "delete"
	case rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE:
		return "activate"
	default:
		return "add"
	}
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return v
}
