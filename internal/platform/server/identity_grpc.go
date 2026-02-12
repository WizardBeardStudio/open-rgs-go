package server

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"golang.org/x/crypto/bcrypt"
)

var errIdentityPersistenceRequired = errors.New("identity persistence required")

type identitySession struct {
	refreshToken string
	actorID      string
	actorType    rgsv1.ActorType
	expiresAt    time.Time
	revoked      bool
}

type IdentityService struct {
	rgsv1.UnimplementedIdentityServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	mu              sync.Mutex
	refreshSessions map[string]*identitySession
	failedAttempts  map[string]int
	lockedUntil     map[string]time.Time
	nextAuditID     int64
	tokenSigner     *platformauth.JWTSigner
	accessTTL       time.Duration
	refreshTTL      time.Duration
	lockoutTTL      time.Duration
	maxFailures     int
	db              *sql.DB
}

func NewIdentityService(clk clock.Clock, signingSecret string, accessTTL, refreshTTL time.Duration, db ...*sql.DB) *IdentityService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	if accessTTL <= 0 {
		accessTTL = 15 * time.Minute
	}
	if refreshTTL <= 0 {
		refreshTTL = 24 * time.Hour
	}
	if signingSecret == "" {
		signingSecret = "dev-insecure-change-me"
	}
	return &IdentityService{
		Clock:           clk,
		AuditStore:      audit.NewInMemoryStore(),
		refreshSessions: make(map[string]*identitySession),
		failedAttempts:  make(map[string]int),
		lockedUntil:     make(map[string]time.Time),
		tokenSigner:     platformauth.NewJWTSigner(signingSecret),
		accessTTL:       accessTTL,
		refreshTTL:      refreshTTL,
		lockoutTTL:      15 * time.Minute,
		maxFailures:     5,
		db:              handle,
	}
}

func (s *IdentityService) SetJWTSigner(signer *platformauth.JWTSigner) {
	if s == nil || signer == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenSigner = signer
}

func (s *IdentityService) SetLockoutPolicy(maxFailures int, ttl time.Duration) {
	if s == nil {
		return
	}
	if maxFailures <= 0 {
		maxFailures = 5
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maxFailures = maxFailures
	s.lockoutTTL = ttl
}

func (s *IdentityService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *IdentityService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *IdentityService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "identity-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *IdentityService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
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
		ObjectType:   "identity_session",
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

func (s *IdentityService) auditDenied(meta *rgsv1.RequestMeta, objectID, action, reason string) {
	_ = s.appendAudit(meta, objectID, action, []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
}

func (s *IdentityService) validateLoginRequest(req *rgsv1.LoginRequest) (string, rgsv1.ActorType, string) {
	if req == nil || req.Meta == nil || req.Meta.Actor == nil {
		return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "actor is required"
	}
	if req.Meta.Actor.ActorId == "" || req.Meta.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "actor binding is required"
	}

	switch creds := req.Credentials.(type) {
	case *rgsv1.LoginRequest_Player:
		if creds.Player == nil || creds.Player.PlayerId == "" || creds.Player.Pin == "" {
			return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "player_id and pin are required"
		}
		if req.Meta.Actor.ActorType != rgsv1.ActorType_ACTOR_TYPE_PLAYER || req.Meta.Actor.ActorId != creds.Player.PlayerId {
			return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "actor must match player credentials"
		}
		return creds.Player.PlayerId, rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""
	case *rgsv1.LoginRequest_Operator:
		if creds.Operator == nil || creds.Operator.OperatorId == "" || creds.Operator.Password == "" {
			return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "operator_id and password are required"
		}
		if req.Meta.Actor.ActorType != rgsv1.ActorType_ACTOR_TYPE_OPERATOR || req.Meta.Actor.ActorId != creds.Operator.OperatorId {
			return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "actor must match operator credentials"
		}
		return creds.Operator.OperatorId, rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""
	default:
		return "", rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED, "credentials are required"
	}
}

func (s *IdentityService) signAccessToken(actorID string, actorType rgsv1.ActorType) (string, string, error) {
	now := s.now()
	signed, expiresAt, err := s.tokenSigner.SignActor(platformauth.Actor{
		ID:   actorID,
		Type: actorType.String(),
	}, now, s.accessTTL)
	if err != nil {
		return "", "", err
	}
	return signed, expiresAt.Format(time.RFC3339Nano), nil
}

func randomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func sessionSnapshot(refreshToken, actorID string, actorType rgsv1.ActorType, expiresAt time.Time, revoked bool) []byte {
	payload := map[string]any{
		"refresh_token": refreshToken,
		"actor_id":      actorID,
		"actor_type":    actorType.String(),
		"expires_at":    expiresAt.UTC().Format(time.RFC3339Nano),
		"revoked":       revoked,
	}
	b, _ := json.Marshal(payload)
	return b
}

func lockKey(actorID string, actorType rgsv1.ActorType) string {
	return actorType.String() + "|" + actorID
}

func (s *IdentityService) checkLocked(ctx context.Context, actorID string, actorType rgsv1.ActorType) (bool, error) {
	if s.db != nil {
		const q = `
SELECT COALESCE(locked_until, to_timestamp(0))
FROM identity_lockouts
WHERE actor_id = $1 AND actor_type = $2
`
		var lockedUntil time.Time
		err := s.db.QueryRowContext(ctx, q, actorID, actorType.String()).Scan(&lockedUntil)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return lockedUntil.After(s.now()), nil
	}
	until := s.lockedUntil[lockKey(actorID, actorType)]
	return until.After(s.now()), nil
}

func (s *IdentityService) recordFailure(ctx context.Context, actorID string, actorType rgsv1.ActorType) error {
	if s.db != nil {
		const q = `
INSERT INTO identity_lockouts (actor_id, actor_type, failed_attempts, locked_until)
VALUES ($1, $2, 1, NULL)
ON CONFLICT (actor_id, actor_type) DO UPDATE
SET failed_attempts = identity_lockouts.failed_attempts + 1,
    locked_until = CASE
      WHEN identity_lockouts.failed_attempts + 1 >= $3
      THEN NOW() + ($4 || ' seconds')::interval
      ELSE identity_lockouts.locked_until
    END,
    updated_at = NOW()
`
		_, err := s.db.ExecContext(ctx, q, actorID, actorType.String(), s.maxFailures, int(s.lockoutTTL.Seconds()))
		return err
	}
	k := lockKey(actorID, actorType)
	s.failedAttempts[k]++
	if s.failedAttempts[k] >= s.maxFailures {
		s.lockedUntil[k] = s.now().Add(s.lockoutTTL)
	}
	return nil
}

func (s *IdentityService) resetFailures(ctx context.Context, actorID string, actorType rgsv1.ActorType) error {
	if s.db != nil {
		const q = `
INSERT INTO identity_lockouts (actor_id, actor_type, failed_attempts, locked_until)
VALUES ($1, $2, 0, NULL)
ON CONFLICT (actor_id, actor_type) DO UPDATE
SET failed_attempts = 0,
    locked_until = NULL,
    updated_at = NOW()
`
		_, err := s.db.ExecContext(ctx, q, actorID, actorType.String())
		return err
	}
	k := lockKey(actorID, actorType)
	delete(s.failedAttempts, k)
	delete(s.lockedUntil, k)
	return nil
}

func (s *IdentityService) verifyCredentials(ctx context.Context, actorID string, actorType rgsv1.ActorType, secret string) (bool, error) {
	if s.db != nil {
		const q = `
SELECT password_hash, status
FROM identity_credentials
WHERE actor_id = $1 AND actor_type = $2
`
		var hash, status string
		err := s.db.QueryRowContext(ctx, q, actorID, actorType.String()).Scan(&hash, &status)
		if err == sql.ErrNoRows {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if status != "active" {
			return false, nil
		}
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret)) == nil, nil
	}
	if actorType == rgsv1.ActorType_ACTOR_TYPE_PLAYER {
		return secret == "1234", nil
	}
	if actorType == rgsv1.ActorType_ACTOR_TYPE_OPERATOR {
		return secret == "operator-pass", nil
	}
	return false, nil
}

func (s *IdentityService) setCredentialHash(ctx context.Context, actorID string, actorType rgsv1.ActorType, hash string) error {
	if s.db == nil {
		return errIdentityPersistenceRequired
	}
	const q = `
INSERT INTO identity_credentials (actor_id, actor_type, password_hash, status, updated_at)
VALUES ($1, $2, $3, 'active', NOW())
ON CONFLICT (actor_id, actor_type) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    status = 'active',
    updated_at = NOW()
`
	_, err := s.db.ExecContext(ctx, q, actorID, actorType.String(), hash)
	return err
}

func (s *IdentityService) Login(ctx context.Context, req *rgsv1.LoginRequest) (*rgsv1.LoginResponse, error) {
	actorID, actorType, reason := s.validateLoginRequest(req)
	if reason != "" {
		var meta *rgsv1.RequestMeta
		if req != nil {
			meta = req.Meta
		}
		s.auditDenied(meta, "", "identity_login", reason)
		return &rgsv1.LoginResponse{Meta: s.responseMeta(meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	secret := ""
	switch creds := req.Credentials.(type) {
	case *rgsv1.LoginRequest_Player:
		secret = creds.Player.GetPin()
	case *rgsv1.LoginRequest_Operator:
		secret = creds.Operator.GetPassword()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	locked, err := s.checkLocked(ctx, actorID, actorType)
	if err != nil {
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if locked {
		s.auditDenied(req.Meta, "", "identity_login", "account locked")
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "account locked")}, nil
	}

	okCreds, err := s.verifyCredentials(ctx, actorID, actorType, secret)
	if err != nil {
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if !okCreds {
		_ = s.recordFailure(ctx, actorID, actorType)
		s.auditDenied(req.Meta, "", "identity_login", "invalid credentials")
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "invalid credentials")}, nil
	}
	if err := s.resetFailures(ctx, actorID, actorType); err != nil {
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	accessToken, accessExpiry, err := s.signAccessToken(actorID, actorType)
	if err != nil {
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to sign token")}, nil
	}
	refreshToken, err := randomToken()
	if err != nil {
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to create refresh token")}, nil
	}

	expiresAt := s.now().Add(s.refreshTTL)
	sess := &identitySession{
		refreshToken: refreshToken,
		actorID:      actorID,
		actorType:    actorType,
		expiresAt:    expiresAt,
	}
	if s.db != nil {
		if err := s.storeSession(ctx, sess); err != nil {
			return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		s.refreshSessions[refreshToken] = sess
	}
	if err := s.appendAudit(req.Meta, refreshToken, "identity_login", []byte(`{}`), sessionSnapshot(refreshToken, actorID, actorType, expiresAt, false), audit.ResultSuccess, ""); err != nil {
		if s.db != nil {
			_ = s.revokeSession(ctx, refreshToken)
		} else {
			delete(s.refreshSessions, refreshToken)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}

	return &rgsv1.LoginResponse{
		Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Token: &rgsv1.SessionToken{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			TokenType:    "Bearer",
			ExpiresAt:    accessExpiry,
			Actor:        &rgsv1.Actor{ActorId: actorID, ActorType: actorType},
		},
	}, nil
}

func (s *IdentityService) Logout(ctx context.Context, req *rgsv1.LogoutRequest) (*rgsv1.LogoutResponse, error) {
	if req == nil || req.RefreshToken == "" {
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "refresh_token is required")}, nil
	}
	if req.Meta == nil || req.Meta.Actor == nil || req.Meta.Actor.ActorId == "" || req.Meta.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var sess *identitySession
	if s.db != nil {
		var err error
		sess, err = s.getSession(ctx, req.RefreshToken)
		if err != nil {
			return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		sess = s.refreshSessions[req.RefreshToken]
	}
	if sess == nil {
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "refresh token not found")}, nil
	}
	if sess.actorID != req.Meta.Actor.ActorId || sess.actorType != req.Meta.Actor.ActorType {
		s.auditDenied(req.Meta, req.RefreshToken, "identity_logout", "actor mismatch")
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "actor mismatch")}, nil
	}

	before := sessionSnapshot(sess.refreshToken, sess.actorID, sess.actorType, sess.expiresAt, sess.revoked)
	sess.revoked = true
	if s.db != nil {
		if err := s.revokeSession(ctx, req.RefreshToken); err != nil {
			return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		delete(s.refreshSessions, req.RefreshToken)
	}
	after := sessionSnapshot(sess.refreshToken, sess.actorID, sess.actorType, sess.expiresAt, true)
	if err := s.appendAudit(req.Meta, req.RefreshToken, "identity_logout", before, after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, "")}, nil
}

func (s *IdentityService) RefreshToken(ctx context.Context, req *rgsv1.RefreshTokenRequest) (*rgsv1.RefreshTokenResponse, error) {
	if req == nil || req.RefreshToken == "" {
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "refresh_token is required")}, nil
	}
	if req.Meta == nil || req.Meta.Actor == nil || req.Meta.Actor.ActorId == "" || req.Meta.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var sess *identitySession
	if s.db != nil {
		var err error
		sess, err = s.getSession(ctx, req.RefreshToken)
		if err != nil {
			return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		sess = s.refreshSessions[req.RefreshToken]
	}
	if sess == nil || sess.revoked || !sess.expiresAt.After(s.now()) {
		s.auditDenied(req.Meta, req.RefreshToken, "identity_refresh", "invalid refresh token")
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "invalid refresh token")}, nil
	}
	if sess.actorID != req.Meta.Actor.ActorId || sess.actorType != req.Meta.Actor.ActorType {
		s.auditDenied(req.Meta, req.RefreshToken, "identity_refresh", "actor mismatch")
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "actor mismatch")}, nil
	}

	accessToken, accessExpiry, err := s.signAccessToken(sess.actorID, sess.actorType)
	if err != nil {
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to sign token")}, nil
	}
	newRefreshToken, err := randomToken()
	if err != nil {
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to create refresh token")}, nil
	}

	before := sessionSnapshot(sess.refreshToken, sess.actorID, sess.actorType, sess.expiresAt, sess.revoked)
	newExpiry := s.now().Add(s.refreshTTL)
	next := &identitySession{
		refreshToken: newRefreshToken,
		actorID:      sess.actorID,
		actorType:    sess.actorType,
		expiresAt:    newExpiry,
	}
	if s.db != nil {
		if err := s.rotateSession(ctx, req.RefreshToken, next); err != nil {
			return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
	} else {
		delete(s.refreshSessions, req.RefreshToken)
		s.refreshSessions[newRefreshToken] = next
	}
	after := sessionSnapshot(newRefreshToken, sess.actorID, sess.actorType, newExpiry, false)
	if err := s.appendAudit(req.Meta, newRefreshToken, "identity_refresh", before, after, audit.ResultSuccess, ""); err != nil {
		if s.db == nil {
			delete(s.refreshSessions, newRefreshToken)
		}
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}

	return &rgsv1.RefreshTokenResponse{
		Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""),
		Token: &rgsv1.SessionToken{
			AccessToken:  accessToken,
			RefreshToken: newRefreshToken,
			TokenType:    "Bearer",
			ExpiresAt:    accessExpiry,
			Actor:        &rgsv1.Actor{ActorId: sess.actorID, ActorType: sess.actorType},
		},
	}, nil
}

func (s *IdentityService) SetCredential(ctx context.Context, req *rgsv1.SetCredentialRequest) (*rgsv1.SetCredentialResponse, error) {
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED || req.Secret == "" {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor and secret are required")}, nil
	}
	authActor, reason := resolveActor(ctx, req.Meta)
	if reason != "" {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if authActor.ActorType != rgsv1.ActorType_ACTOR_TYPE_OPERATOR && authActor.ActorType != rgsv1.ActorType_ACTOR_TYPE_SERVICE {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "unauthorized actor type")}, nil
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Secret), bcrypt.DefaultCost)
	if err != nil {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to hash secret")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.setCredentialHash(ctx, req.Actor.ActorId, req.Actor.ActorType, string(hash)); err != nil {
		if errors.Is(err, errIdentityPersistenceRequired) {
			return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "credential management requires database")}, nil
		}
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	if err := s.resetFailures(ctx, req.Actor.ActorId, req.Actor.ActorType); err != nil {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	after := map[string]any{
		"actor_id":   req.Actor.ActorId,
		"actor_type": req.Actor.ActorType.String(),
		"status":     "active",
	}
	afterJSON, _ := json.Marshal(after)
	if err := s.appendAudit(req.Meta, req.Actor.ActorId, "identity_set_credential", []byte(`{}`), afterJSON, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, "")}, nil
}
