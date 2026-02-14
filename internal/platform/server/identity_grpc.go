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

type loginRateWindow struct {
	start time.Time
	count int
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
	loginRateMax    int
	loginRateWindow time.Duration
	loginRates      map[string]loginRateWindow
	db              *sql.DB
	onLogin         func(result rgsv1.ResultCode, actorType rgsv1.ActorType)
	onLockout       func(actorType rgsv1.ActorType)
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
		loginRateMax:    60,
		loginRateWindow: time.Minute,
		loginRates:      make(map[string]loginRateWindow),
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

func (s *IdentityService) SetMetricsObservers(onLogin func(result rgsv1.ResultCode, actorType rgsv1.ActorType), onLockout func(actorType rgsv1.ActorType)) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLogin = onLogin
	s.onLockout = onLockout
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

func (s *IdentityService) SetLoginRateLimit(maxAttempts int, window time.Duration) {
	if s == nil {
		return
	}
	if maxAttempts < 0 {
		maxAttempts = 0
	}
	if window <= 0 {
		window = time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loginRateMax = maxAttempts
	s.loginRateWindow = window
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
	ev := audit.Event{
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
	}
	if s.db != nil {
		if err := appendAuditEventToDB(context.Background(), s.db, ev); err != nil {
			return err
		}
	}
	_, err := s.AuditStore.Append(ev)
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

func loginRateKey(actorID string, actorType rgsv1.ActorType) string {
	return "login_rate|" + lockKey(actorID, actorType)
}

func (s *IdentityService) rateLimitExceeded(ctx context.Context, actorID string, actorType rgsv1.ActorType) (bool, error) {
	if s.loginRateMax <= 0 {
		return false, nil
	}
	if s.db != nil {
		const q = `
INSERT INTO identity_login_rate_limits (actor_id, actor_type, window_start, attempt_count, updated_at)
VALUES ($1, $2, NOW(), 1, NOW())
ON CONFLICT (actor_id, actor_type) DO UPDATE
SET window_start = CASE
      WHEN identity_login_rate_limits.window_start <= NOW() - ($3 || ' seconds')::interval
      THEN NOW()
      ELSE identity_login_rate_limits.window_start
    END,
    attempt_count = CASE
      WHEN identity_login_rate_limits.window_start <= NOW() - ($3 || ' seconds')::interval
      THEN 1
      ELSE identity_login_rate_limits.attempt_count + 1
    END,
    updated_at = NOW()
RETURNING attempt_count
`
		var attempts int
		if err := s.db.QueryRowContext(ctx, q, actorID, actorType.String(), int(s.loginRateWindow.Seconds())).Scan(&attempts); err != nil {
			return false, err
		}
		return attempts > s.loginRateMax, nil
	}
	key := loginRateKey(actorID, actorType)
	now := s.now()
	window := s.loginRates[key]
	if window.start.IsZero() || now.Sub(window.start) >= s.loginRateWindow {
		window = loginRateWindow{start: now, count: 0}
	}
	window.count++
	s.loginRates[key] = window
	return window.count > s.loginRateMax, nil
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

func (s *IdentityService) recordFailure(ctx context.Context, actorID string, actorType rgsv1.ActorType) (bool, error) {
	if s.db != nil {
		wasLocked, err := s.checkLocked(ctx, actorID, actorType)
		if err != nil {
			return false, err
		}
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
		_, err = s.db.ExecContext(ctx, q, actorID, actorType.String(), s.maxFailures, int(s.lockoutTTL.Seconds()))
		if err != nil {
			return false, err
		}
		nowLocked, err := s.checkLocked(ctx, actorID, actorType)
		if err != nil {
			return false, err
		}
		return !wasLocked && nowLocked, nil
	}
	k := lockKey(actorID, actorType)
	beforeLocked := s.lockedUntil[k].After(s.now())
	s.failedAttempts[k]++
	if s.failedAttempts[k] >= s.maxFailures {
		s.lockedUntil[k] = s.now().Add(s.lockoutTTL)
	}
	afterLocked := s.lockedUntil[k].After(s.now())
	return !beforeLocked && afterLocked, nil
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

func (s *IdentityService) authorizeIdentityAdmin(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	if actor.ActorType != rgsv1.ActorType_ACTOR_TYPE_OPERATOR && actor.ActorType != rgsv1.ActorType_ACTOR_TYPE_SERVICE {
		return false, "unauthorized actor type"
	}
	return true, ""
}

func (s *IdentityService) Login(ctx context.Context, req *rgsv1.LoginRequest) (*rgsv1.LoginResponse, error) {
	actorID, actorType, reason := s.validateLoginRequest(req)
	if reason != "" {
		var meta *rgsv1.RequestMeta
		if req != nil {
			meta = req.Meta
		}
		s.auditDenied(meta, "", "identity_login", reason)
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_DENIED, rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED)
		}
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

	exceeded, err := s.rateLimitExceeded(ctx, actorID, actorType)
	if err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if exceeded {
		s.auditDenied(req.Meta, "", "identity_login", "rate limit exceeded")
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_DENIED, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "rate limit exceeded")}, nil
	}

	locked, err := s.checkLocked(ctx, actorID, actorType)
	if err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if locked {
		s.auditDenied(req.Meta, "", "identity_login", "account locked")
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_DENIED, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "account locked")}, nil
	}

	okCreds, err := s.verifyCredentials(ctx, actorID, actorType, secret)
	if err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if !okCreds {
		lockedNow, _ := s.recordFailure(ctx, actorID, actorType)
		if lockedNow && s.onLockout != nil {
			s.onLockout(actorType)
		}
		s.auditDenied(req.Meta, "", "identity_login", "invalid credentials")
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_DENIED, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "invalid credentials")}, nil
	}
	if err := s.resetFailures(ctx, actorID, actorType); err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	accessToken, accessExpiry, err := s.signAccessToken(actorID, actorType)
	if err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to sign token")}, nil
	}
	refreshToken, err := randomToken()
	if err != nil {
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
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
			if s.onLogin != nil {
				s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
			}
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
		if s.onLogin != nil {
			s.onLogin(rgsv1.ResultCode_RESULT_CODE_ERROR, actorType)
		}
		return &rgsv1.LoginResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if s.onLogin != nil {
		s.onLogin(rgsv1.ResultCode_RESULT_CODE_OK, actorType)
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
		s.auditDenied(req.Meta, req.RefreshToken, "identity_logout", "actor mismatch with token")
		return &rgsv1.LogoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "actor mismatch with token")}, nil
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
		s.auditDenied(req.Meta, req.RefreshToken, "identity_refresh", "actor mismatch with token")
		return &rgsv1.RefreshTokenResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "actor mismatch with token")}, nil
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
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED || req.CredentialHash == "" {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor and credential hash are required")}, nil
	}
	if ok, reason := s.authorizeIdentityAdmin(ctx, req.Meta); !ok {
		s.auditDenied(req.Meta, req.Actor.ActorId, "identity_set_credential", reason)
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	cost, err := bcrypt.Cost([]byte(req.CredentialHash))
	if err != nil {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "credential hash must be bcrypt")}, nil
	}
	if cost < bcrypt.DefaultCost {
		return &rgsv1.SetCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "credential hash cost too low")}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.setCredentialHash(ctx, req.Actor.ActorId, req.Actor.ActorType, req.CredentialHash); err != nil {
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

func (s *IdentityService) DisableCredential(ctx context.Context, req *rgsv1.DisableCredentialRequest) (*rgsv1.DisableCredentialResponse, error) {
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}
	if ok, reason := s.authorizeIdentityAdmin(ctx, req.Meta); !ok {
		s.auditDenied(req.Meta, req.Actor.ActorId, "identity_disable_credential", reason)
		return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	updated, err := s.setCredentialStatus(ctx, req.Actor.ActorId, req.Actor.ActorType, "disabled")
	if err != nil {
		if errors.Is(err, errIdentityPersistenceRequired) {
			return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "credential management requires database")}, nil
		}
		return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if !updated {
		return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "credential not found")}, nil
	}
	if err := s.appendAudit(req.Meta, req.Actor.ActorId, "identity_disable_credential", []byte(`{}`), []byte(`{"status":"disabled"}`), audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.DisableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, "")}, nil
}

func (s *IdentityService) EnableCredential(ctx context.Context, req *rgsv1.EnableCredentialRequest) (*rgsv1.EnableCredentialResponse, error) {
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}
	if ok, reason := s.authorizeIdentityAdmin(ctx, req.Meta); !ok {
		s.auditDenied(req.Meta, req.Actor.ActorId, "identity_enable_credential", reason)
		return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	updated, err := s.setCredentialStatus(ctx, req.Actor.ActorId, req.Actor.ActorType, "active")
	if err != nil {
		if errors.Is(err, errIdentityPersistenceRequired) {
			return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, "credential management requires database")}, nil
		}
		return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	if !updated {
		return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "credential not found")}, nil
	}
	if err := s.appendAudit(req.Meta, req.Actor.ActorId, "identity_enable_credential", []byte(`{}`), []byte(`{"status":"active"}`), audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.EnableCredentialResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, "")}, nil
}

func (s *IdentityService) lockoutStatus(ctx context.Context, actor *rgsv1.Actor) (*rgsv1.LockoutStatus, error) {
	if actor == nil {
		return nil, nil
	}
	if s.db != nil {
		failed, lockedUntil, err := s.getLockoutStatusDB(ctx, actor.ActorId, actor.ActorType)
		if err != nil {
			return nil, err
		}
		out := &rgsv1.LockoutStatus{
			Actor:          &rgsv1.Actor{ActorId: actor.ActorId, ActorType: actor.ActorType},
			FailedAttempts: int32(failed),
			Locked:         lockedUntil != nil && lockedUntil.After(s.now()),
		}
		if lockedUntil != nil {
			out.LockedUntil = lockedUntil.UTC().Format(time.RFC3339Nano)
		}
		return out, nil
	}
	k := lockKey(actor.ActorId, actor.ActorType)
	until := s.lockedUntil[k]
	out := &rgsv1.LockoutStatus{
		Actor:          &rgsv1.Actor{ActorId: actor.ActorId, ActorType: actor.ActorType},
		FailedAttempts: int32(s.failedAttempts[k]),
		Locked:         until.After(s.now()),
	}
	if !until.IsZero() {
		out.LockedUntil = until.UTC().Format(time.RFC3339Nano)
	}
	return out, nil
}

func (s *IdentityService) GetLockout(ctx context.Context, req *rgsv1.GetLockoutRequest) (*rgsv1.GetLockoutResponse, error) {
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.GetLockoutResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}
	if ok, reason := s.authorizeIdentityAdmin(ctx, req.Meta); !ok {
		s.auditDenied(req.Meta, req.Actor.ActorId, "identity_get_lockout", reason)
		return &rgsv1.GetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	status, err := s.lockoutStatus(ctx, req.Actor)
	if err != nil {
		return &rgsv1.GetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	return &rgsv1.GetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Status: status}, nil
}

func (s *IdentityService) ResetLockout(ctx context.Context, req *rgsv1.ResetLockoutRequest) (*rgsv1.ResetLockoutResponse, error) {
	if req == nil || req.Actor == nil || req.Actor.ActorId == "" || req.Actor.ActorType == rgsv1.ActorType_ACTOR_TYPE_UNSPECIFIED {
		return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "actor is required")}, nil
	}
	if ok, reason := s.authorizeIdentityAdmin(ctx, req.Meta); !ok {
		s.auditDenied(req.Meta, req.Actor.ActorId, "identity_reset_lockout", reason)
		return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.resetFailures(ctx, req.Actor.ActorId, req.Actor.ActorType); err != nil {
		return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	status, err := s.lockoutStatus(ctx, req.Actor)
	if err != nil {
		return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}
	afterJSON, _ := json.Marshal(status)
	if err := s.appendAudit(req.Meta, req.Actor.ActorId, "identity_reset_lockout", []byte(`{}`), afterJSON, audit.ResultSuccess, req.Reason); err != nil {
		return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	return &rgsv1.ResetLockoutResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), Status: status}, nil
}
