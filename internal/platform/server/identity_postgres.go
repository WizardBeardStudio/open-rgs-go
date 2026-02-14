package server

import (
	"context"
	"database/sql"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
)

func (s *IdentityService) HasActiveCredentials(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}
	const q = `
SELECT COUNT(*)
FROM identity_credentials
WHERE status = 'active'
`
	var count int64
	if err := s.db.QueryRowContext(ctx, q).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *IdentityService) setCredentialStatus(ctx context.Context, actorID string, actorType rgsv1.ActorType, status string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errIdentityPersistenceRequired
	}
	const q = `
UPDATE identity_credentials
SET status = $3, updated_at = NOW()
WHERE actor_id = $1 AND actor_type = $2
`
	res, err := s.db.ExecContext(ctx, q, actorID, actorType.String(), status)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (s *IdentityService) getLockoutStatusDB(ctx context.Context, actorID string, actorType rgsv1.ActorType) (int, *time.Time, error) {
	if s == nil || s.db == nil {
		return 0, nil, nil
	}
	const q = `
SELECT failed_attempts, locked_until
FROM identity_lockouts
WHERE actor_id = $1 AND actor_type = $2
`
	var failed int
	var locked sql.NullTime
	err := s.db.QueryRowContext(ctx, q, actorID, actorType.String()).Scan(&failed, &locked)
	if err == sql.ErrNoRows {
		return 0, nil, nil
	}
	if err != nil {
		return 0, nil, err
	}
	if locked.Valid {
		t := locked.Time.UTC()
		return failed, &t, nil
	}
	return failed, nil, nil
}

func (s *IdentityService) storeSession(ctx context.Context, sess *identitySession) error {
	if s == nil || s.db == nil || sess == nil {
		return nil
	}
	const q = `
INSERT INTO identity_sessions (refresh_token, actor_id, actor_type, expires_at, revoked)
VALUES ($1, $2, $3, $4::timestamptz, $5)
ON CONFLICT (refresh_token) DO UPDATE SET
  actor_id = EXCLUDED.actor_id,
  actor_type = EXCLUDED.actor_type,
  expires_at = EXCLUDED.expires_at,
  revoked = EXCLUDED.revoked,
  updated_at = NOW()
`
	_, err := s.db.ExecContext(ctx, q, sess.refreshToken, sess.actorID, sess.actorType.String(), sess.expiresAt.UTC().Format(time.RFC3339Nano), sess.revoked)
	return err
}

func (s *IdentityService) getSession(ctx context.Context, refreshToken string) (*identitySession, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT refresh_token, actor_id, actor_type, expires_at, revoked
FROM identity_sessions
WHERE refresh_token = $1
`
	var sess identitySession
	var actorType string
	err := s.db.QueryRowContext(ctx, q, refreshToken).Scan(&sess.refreshToken, &sess.actorID, &actorType, &sess.expiresAt, &sess.revoked)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sess.actorType = actorTypeFromString(actorType)
	return &sess, nil
}

func (s *IdentityService) revokeSession(ctx context.Context, refreshToken string) error {
	if s == nil || s.db == nil {
		return nil
	}
	const q = `
UPDATE identity_sessions
SET revoked = TRUE, updated_at = NOW()
WHERE refresh_token = $1
`
	_, err := s.db.ExecContext(ctx, q, refreshToken)
	return err
}

func (s *IdentityService) rotateSession(ctx context.Context, oldRefreshToken string, next *identitySession) error {
	if s == nil || s.db == nil || next == nil {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	const revokeQ = `
UPDATE identity_sessions
SET revoked = TRUE, updated_at = NOW()
WHERE refresh_token = $1
`
	if _, err := tx.ExecContext(ctx, revokeQ, oldRefreshToken); err != nil {
		return err
	}
	const insertQ = `
INSERT INTO identity_sessions (refresh_token, actor_id, actor_type, expires_at, revoked)
VALUES ($1, $2, $3, $4::timestamptz, $5)
`
	if _, err := tx.ExecContext(ctx, insertQ, next.refreshToken, next.actorID, next.actorType.String(), next.expiresAt.UTC().Format(time.RFC3339Nano), next.revoked); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *IdentityService) CleanupExpiredSessions(ctx context.Context, batchSize int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	const q = `
WITH doomed AS (
  SELECT ctid
  FROM identity_sessions
  WHERE expires_at <= NOW()
  ORDER BY expires_at ASC
  LIMIT $1
)
DELETE FROM identity_sessions
WHERE ctid IN (SELECT ctid FROM doomed)
`
	res, err := s.db.ExecContext(ctx, q, batchSize)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *IdentityService) StartSessionCleanupWorker(ctx context.Context, interval time.Duration, batchSize int, logger func(string, ...any)) {
	if s == nil || s.db == nil || interval <= 0 {
		return
	}
	if batchSize <= 0 {
		batchSize = 500
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for {
					deleted, err := s.CleanupExpiredSessions(ctx, batchSize)
					if err != nil {
						if logger != nil {
							logger("identity session cleanup failed: %v", err)
						}
						break
					}
					if deleted == 0 {
						break
					}
					if logger != nil {
						logger("identity session cleanup removed %d expired sessions", deleted)
					}
					if deleted < int64(batchSize) {
						break
					}
				}
			}
		}
	}()
}
