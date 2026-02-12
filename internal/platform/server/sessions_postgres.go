package server

import (
	"context"
	"database/sql"
	"errors"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func sessionStateToDB(state rgsv1.SessionState) string {
	switch state {
	case rgsv1.SessionState_SESSION_STATE_ACTIVE:
		return "ACTIVE"
	case rgsv1.SessionState_SESSION_STATE_ENDED:
		return "ENDED"
	case rgsv1.SessionState_SESSION_STATE_EXPIRED:
		return "EXPIRED"
	default:
		return "UNSPECIFIED"
	}
}

func sessionStateFromDB(raw string) rgsv1.SessionState {
	switch raw {
	case "ACTIVE":
		return rgsv1.SessionState_SESSION_STATE_ACTIVE
	case "ENDED":
		return rgsv1.SessionState_SESSION_STATE_ENDED
	case "EXPIRED":
		return rgsv1.SessionState_SESSION_STATE_EXPIRED
	default:
		return rgsv1.SessionState_SESSION_STATE_UNSPECIFIED
	}
}

func (s *SessionsService) upsertSessionInDB(ctx context.Context, sess *rgsv1.PlayerSession) error {
	if s == nil || s.db == nil || sess == nil {
		return nil
	}
	const q = `
INSERT INTO player_sessions (
  session_id, player_id, device_id, state, started_at, last_seen_at, ended_at, expires_at, end_reason, created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5::timestamptz,$6::timestamptz,NULLIF($7,'')::timestamptz,$8::timestamptz,$9,NOW(),NOW())
ON CONFLICT (session_id) DO UPDATE SET
  player_id = EXCLUDED.player_id,
  device_id = EXCLUDED.device_id,
  state = EXCLUDED.state,
  started_at = EXCLUDED.started_at,
  last_seen_at = EXCLUDED.last_seen_at,
  ended_at = EXCLUDED.ended_at,
  expires_at = EXCLUDED.expires_at,
  end_reason = EXCLUDED.end_reason,
  updated_at = NOW()
`
	_, err := s.db.ExecContext(ctx, q,
		sess.SessionId,
		sess.PlayerId,
		sess.DeviceId,
		sessionStateToDB(sess.State),
		nonEmptyTime(sess.StartedAt),
		nonEmptyTime(sess.LastSeenAt),
		sess.EndedAt,
		nonEmptyTime(sess.ExpiresAt),
		sess.EndReason,
	)
	return err
}

func (s *SessionsService) getSessionFromDB(ctx context.Context, sessionID string) (*rgsv1.PlayerSession, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	const q = `
SELECT session_id, player_id, device_id, state, started_at, last_seen_at, ended_at, expires_at, end_reason
FROM player_sessions
WHERE session_id = $1
`
	var (
		sess                             rgsv1.PlayerSession
		stateRaw                         string
		startedAt, lastSeenAt, expiresAt time.Time
		endedAt                          *time.Time
	)
	err := s.db.QueryRowContext(ctx, q, sessionID).Scan(
		&sess.SessionId,
		&sess.PlayerId,
		&sess.DeviceId,
		&stateRaw,
		&startedAt,
		&lastSeenAt,
		&endedAt,
		&expiresAt,
		&sess.EndReason,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	sess.State = sessionStateFromDB(stateRaw)
	sess.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	sess.LastSeenAt = lastSeenAt.UTC().Format(time.RFC3339Nano)
	if endedAt != nil {
		sess.EndedAt = endedAt.UTC().Format(time.RFC3339Nano)
	}
	sess.ExpiresAt = expiresAt.UTC().Format(time.RFC3339Nano)
	return &sess, nil
}
