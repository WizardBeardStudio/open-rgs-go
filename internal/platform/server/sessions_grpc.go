package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type SessionsService struct {
	rgsv1.UnimplementedSessionsServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu                   sync.Mutex
	sessions             map[string]*rgsv1.PlayerSession
	nextAuditID          int64
	defaultTimeout       time.Duration
	db                   *sql.DB
	disableInMemoryCache bool
}

func NewSessionsService(clk clock.Clock, db ...*sql.DB) *SessionsService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &SessionsService{
		Clock:          clk,
		AuditStore:     audit.NewInMemoryStore(),
		sessions:       make(map[string]*rgsv1.PlayerSession),
		defaultTimeout: time.Hour,
		db:             handle,
	}
}

func (s *SessionsService) SetDisableInMemoryCache(disable bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.disableInMemoryCache = disable
}

func (s *SessionsService) SetDefaultTimeout(timeout time.Duration) {
	if s == nil || timeout <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultTimeout = timeout
}

func (s *SessionsService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *SessionsService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *SessionsService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "sessions-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *SessionsService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
	if s.AuditStore == nil {
		return audit.ErrCorruptChain
	}
	actorID := "system"
	actorType := "service"
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	now := s.now()
	_, err := s.AuditStore.Append(audit.Event{
		AuditID:      s.nextAuditIDLocked(),
		OccurredAt:   now,
		RecordedAt:   now,
		ActorID:      actorID,
		ActorType:    actorType,
		ObjectType:   "player_session",
		ObjectID:     objectID,
		Action:       action,
		Before:       before,
		After:        after,
		Result:       result,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	})
	return err
}

func cloneSession(in *rgsv1.PlayerSession) *rgsv1.PlayerSession {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.PlayerSession)
	return cp
}

func playerSessionSnapshot(sess *rgsv1.PlayerSession) []byte {
	if sess == nil {
		return []byte(`{}`)
	}
	b, _ := json.Marshal(sess)
	return b
}

func (s *SessionsService) timeoutForRequest(req *rgsv1.StartSessionRequest) time.Duration {
	timeout := s.defaultTimeout
	if req != nil && req.SessionTimeoutSeconds > 0 {
		timeout = time.Duration(req.SessionTimeoutSeconds) * time.Second
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	if timeout > 24*time.Hour {
		timeout = 24 * time.Hour
	}
	return timeout
}

func (s *SessionsService) authorizeStart(ctx context.Context, meta *rgsv1.RequestMeta, playerID string) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_PLAYER:
		if actor.ActorId != playerID {
			return false, "player actor must match player_id"
		}
		return true, ""
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func (s *SessionsService) authorizeAccess(ctx context.Context, meta *rgsv1.RequestMeta, sess *rgsv1.PlayerSession) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_PLAYER:
		if sess == nil || actor.ActorId != sess.PlayerId {
			return false, "player actor unauthorized for session"
		}
		return true, ""
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func (s *SessionsService) loadSession(ctx context.Context, sessionID string) (*rgsv1.PlayerSession, error) {
	if s.db != nil {
		return s.getSessionFromDB(ctx, sessionID)
	}
	if s.disableInMemoryCache {
		return nil, nil
	}
	return cloneSession(s.sessions[sessionID]), nil
}

func (s *SessionsService) persistSession(ctx context.Context, sess *rgsv1.PlayerSession) error {
	if s.db != nil {
		return s.upsertSessionInDB(ctx, sess)
	}
	if s.disableInMemoryCache {
		return sql.ErrConnDone
	}
	s.sessions[sess.SessionId] = cloneSession(sess)
	return nil
}

func (s *SessionsService) touchAndExpireSessionIfNeeded(ctx context.Context, sess *rgsv1.PlayerSession) (*rgsv1.PlayerSession, error) {
	if sess == nil {
		return nil, nil
	}
	now := s.now()
	expiresAt, err := time.Parse(time.RFC3339Nano, sess.ExpiresAt)
	if err != nil {
		expiresAt = now
	}
	updated := cloneSession(sess)
	if updated.State == rgsv1.SessionState_SESSION_STATE_ACTIVE && now.After(expiresAt) {
		updated.State = rgsv1.SessionState_SESSION_STATE_EXPIRED
		updated.EndedAt = now.Format(time.RFC3339Nano)
		if updated.EndReason == "" {
			updated.EndReason = "session timeout"
		}
	}
	if updated.State == rgsv1.SessionState_SESSION_STATE_ACTIVE {
		updated.LastSeenAt = now.Format(time.RFC3339Nano)
	}
	if !proto.Equal(updated, sess) {
		if err := s.persistSession(ctx, updated); err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func (s *SessionsService) StartSession(ctx context.Context, req *rgsv1.StartSessionRequest) (*rgsv1.StartSessionResponse, error) {
	if req == nil || req.PlayerId == "" || req.DeviceId == "" {
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "player_id and device_id are required")}, nil
	}
	if ok, reason := s.authorizeStart(ctx, req.Meta, req.PlayerId); !ok {
		_ = s.appendAudit(req.Meta, "", "start_session", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil && s.disableInMemoryCache {
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	rawToken, err := randomToken()
	if err != nil {
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to create session id")}, nil
	}
	now := s.now()
	timeout := s.timeoutForRequest(req)
	sess := &rgsv1.PlayerSession{
		SessionId:  "sess-" + rawToken,
		PlayerId:   req.PlayerId,
		DeviceId:   req.DeviceId,
		State:      rgsv1.SessionState_SESSION_STATE_ACTIVE,
		StartedAt:  now.Format(time.RFC3339Nano),
		LastSeenAt: now.Format(time.RFC3339Nano),
		ExpiresAt:  now.Add(timeout).Format(time.RFC3339Nano),
	}
	if err := s.persistSession(ctx, sess); err != nil {
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.appendAudit(req.Meta, sess.SessionId, "start_session", []byte(`{}`), playerSessionSnapshot(sess), audit.ResultSuccess, ""); err != nil {
		return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.StartSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Session: cloneSession(sess)}, nil
}

func (s *SessionsService) EndSession(ctx context.Context, req *rgsv1.EndSessionRequest) (*rgsv1.EndSessionResponse, error) {
	if req == nil || req.SessionId == "" {
		return &rgsv1.EndSessionResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "session_id is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.loadSession(ctx, req.SessionId)
	if err != nil {
		return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if sess == nil {
		return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "session not found")}, nil
	}
	if ok, reason := s.authorizeAccess(ctx, req.Meta, sess); !ok {
		_ = s.appendAudit(req.Meta, req.SessionId, "end_session", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	before := playerSessionSnapshot(sess)
	updated := cloneSession(sess)
	if updated.State == rgsv1.SessionState_SESSION_STATE_ACTIVE {
		updated.State = rgsv1.SessionState_SESSION_STATE_ENDED
		updated.EndedAt = s.now().Format(time.RFC3339Nano)
		if req.Reason != "" {
			updated.EndReason = req.Reason
		} else {
			updated.EndReason = "ended by actor"
		}
		if err := s.persistSession(ctx, updated); err != nil {
			return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	}
	if err := s.appendAudit(req.Meta, req.SessionId, "end_session", before, playerSessionSnapshot(updated), audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.EndSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Session: updated}, nil
}

func (s *SessionsService) GetSession(ctx context.Context, req *rgsv1.GetSessionRequest) (*rgsv1.GetSessionResponse, error) {
	if req == nil || req.SessionId == "" {
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "session_id is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sess, err := s.loadSession(ctx, req.SessionId)
	if err != nil {
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if sess == nil {
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "session not found")}, nil
	}
	if ok, reason := s.authorizeAccess(ctx, req.Meta, sess); !ok {
		_ = s.appendAudit(req.Meta, req.SessionId, "get_session", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	updated, err := s.touchAndExpireSessionIfNeeded(ctx, sess)
	if err != nil {
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if err := s.appendAudit(req.Meta, req.SessionId, "get_session", []byte(`{}`), playerSessionSnapshot(updated), audit.ResultSuccess, ""); err != nil {
		return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.GetSessionResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Session: updated}, nil
}
