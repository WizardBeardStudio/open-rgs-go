package server

import (
	"context"
	"database/sql"
	"strings"
	"time"

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
  entry_id, library_path, checksum, version, action, changed_by, reason, occurred_at, signer_kid, signature, signature_alg
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8::timestamptz,$9,$10,$11)
ON CONFLICT (entry_id) DO UPDATE SET
  checksum = EXCLUDED.checksum,
  version = EXCLUDED.version,
  action = EXCLUDED.action,
  changed_by = EXCLUDED.changed_by,
  reason = EXCLUDED.reason,
  occurred_at = EXCLUDED.occurred_at,
  signer_kid = EXCLUDED.signer_kid,
  signature = EXCLUDED.signature,
  signature_alg = EXCLUDED.signature_alg
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
		e.SignerKid,
		e.Signature,
		e.SignatureAlg,
	)
	return err
}

func (s *ConfigService) getCurrentValue(ctx context.Context, namespace, key string) (string, error) {
	if s == nil || s.db == nil {
		return "", nil
	}
	const q = `
SELECT value
FROM config_current_values
WHERE config_namespace = $1 AND config_key = $2
`
	var value string
	err := s.db.QueryRowContext(ctx, q, namespace, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return value, nil
}

func (s *ConfigService) getConfigChange(ctx context.Context, changeID string) (*rgsv1.ConfigChange, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT change_id, config_namespace, config_key, proposed_value, previous_value, reason,
       status::text, proposer_id, approver_id, applied_by, created_at, approved_at, applied_at
FROM config_changes
WHERE change_id = $1
`
	var (
		changeIDVal, ns, key, proposed, previous, reason, status, proposer, approver, appliedBy string
		createdAt                                                                               time.Time
		approvedAt, appliedAt                                                                   sql.NullTime
	)
	err := s.db.QueryRowContext(ctx, q, changeID).Scan(
		&changeIDVal, &ns, &key, &proposed, &previous, &reason,
		&status, &proposer, &approver, &appliedBy, &createdAt, &approvedAt, &appliedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	c := &rgsv1.ConfigChange{
		ChangeId:        changeIDVal,
		ConfigNamespace: ns,
		ConfigKey:       key,
		ProposedValue:   proposed,
		PreviousValue:   previous,
		Reason:          reason,
		Status:          configStatusFromDB(status),
		ProposerId:      proposer,
		ApproverId:      approver,
		AppliedBy:       appliedBy,
		CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
	}
	if approvedAt.Valid {
		c.ApprovedAt = approvedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	if appliedAt.Valid {
		c.AppliedAt = appliedAt.Time.UTC().Format(time.RFC3339Nano)
	}
	return c, nil
}

func (s *ConfigService) listConfigHistoryFromDB(ctx context.Context, namespaceFilter string, limit, offset int) ([]*rgsv1.ConfigChange, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT change_id, config_namespace, config_key, proposed_value, previous_value, reason,
       status::text, proposer_id, approver_id, applied_by, created_at, approved_at, applied_at
FROM config_changes
WHERE ($1 = '' OR config_namespace = $1)
ORDER BY created_at DESC, change_id DESC
LIMIT $2 OFFSET $3
`
	rows, err := s.db.QueryContext(ctx, q, namespaceFilter, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.ConfigChange, 0, limit)
	for rows.Next() {
		var (
			changeIDVal, ns, key, proposed, previous, reason, status, proposer, approver, appliedBy string
			createdAt                                                                               time.Time
			approvedAt, appliedAt                                                                   sql.NullTime
		)
		if err := rows.Scan(
			&changeIDVal, &ns, &key, &proposed, &previous, &reason,
			&status, &proposer, &approver, &appliedBy, &createdAt, &approvedAt, &appliedAt,
		); err != nil {
			return nil, err
		}
		item := &rgsv1.ConfigChange{
			ChangeId:        changeIDVal,
			ConfigNamespace: ns,
			ConfigKey:       key,
			ProposedValue:   proposed,
			PreviousValue:   previous,
			Reason:          reason,
			Status:          configStatusFromDB(status),
			ProposerId:      proposer,
			ApproverId:      approver,
			AppliedBy:       appliedBy,
			CreatedAt:       createdAt.UTC().Format(time.RFC3339Nano),
		}
		if approvedAt.Valid {
			item.ApprovedAt = approvedAt.Time.UTC().Format(time.RFC3339Nano)
		}
		if appliedAt.Valid {
			item.AppliedAt = appliedAt.Time.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *ConfigService) listDownloadEntriesFromDB(ctx context.Context, limit, offset int) ([]*rgsv1.DownloadLibraryEntry, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT entry_id, library_path, checksum, version, action::text, changed_by, reason, occurred_at, signer_kid, signature, signature_alg
FROM download_library_changes
ORDER BY occurred_at DESC, entry_id DESC
LIMIT $1 OFFSET $2
`
	rows, err := s.db.QueryContext(ctx, q, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*rgsv1.DownloadLibraryEntry, 0, limit)
	for rows.Next() {
		var (
			entryID, path, checksum, version, action, changedBy, reason, signerKid, signature, signatureAlg string
			occurredAt                                                                                      time.Time
		)
		if err := rows.Scan(&entryID, &path, &checksum, &version, &action, &changedBy, &reason, &occurredAt, &signerKid, &signature, &signatureAlg); err != nil {
			return nil, err
		}
		out = append(out, &rgsv1.DownloadLibraryEntry{
			EntryId:      entryID,
			LibraryPath:  path,
			Checksum:     checksum,
			Version:      version,
			Action:       downloadActionFromDB(action),
			ChangedBy:    changedBy,
			Reason:       reason,
			OccurredAt:   occurredAt.UTC().Format(time.RFC3339Nano),
			SignerKid:    signerKid,
			Signature:    signature,
			SignatureAlg: signatureAlg,
		})
	}
	return out, rows.Err()
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

func configStatusFromDB(v string) rgsv1.ConfigChangeStatus {
	switch v {
	case "proposed":
		return rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_PROPOSED
	case "approved":
		return rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED
	case "applied":
		return rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED
	case "rejected":
		return rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_REJECTED
	default:
		return rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_UNSPECIFIED
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

func downloadActionFromDB(v string) rgsv1.DownloadAction {
	switch v {
	case "add":
		return rgsv1.DownloadAction_DOWNLOAD_ACTION_ADD
	case "update":
		return rgsv1.DownloadAction_DOWNLOAD_ACTION_UPDATE
	case "delete":
		return rgsv1.DownloadAction_DOWNLOAD_ACTION_DELETE
	case "activate":
		return rgsv1.DownloadAction_DOWNLOAD_ACTION_ACTIVATE
	default:
		return rgsv1.DownloadAction_DOWNLOAD_ACTION_UNSPECIFIED
	}
}

func nullIfEmpty(v string) any {
	if strings.TrimSpace(v) == "" {
		return sql.NullString{}
	}
	return v
}
