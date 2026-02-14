package server

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"golang.org/x/crypto/bcrypt"
)

func TestIdentityPlayerLoginRefreshLogout(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 0, 0, 0, time.UTC)}
	svc := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour)
	ctx := context.Background()

	login, err := svc.Login(ctx, &rgsv1.LoginRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-1", Pin: "1234"},
		},
	})
	if err != nil {
		t.Fatalf("login err: %v", err)
	}
	if login.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("login result: got=%v", login.Meta.GetResultCode())
	}
	if login.Token.GetAccessToken() == "" || login.Token.GetRefreshToken() == "" {
		t.Fatalf("expected non-empty tokens")
	}

	refresh, err := svc.RefreshToken(ctx, &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: login.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("refresh err: %v", err)
	}
	if refresh.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("refresh result: got=%v", refresh.Meta.GetResultCode())
	}
	if refresh.Token.GetRefreshToken() == login.Token.GetRefreshToken() {
		t.Fatalf("expected refresh token rotation")
	}

	logout, err := svc.Logout(ctx, &rgsv1.LogoutRequest{
		Meta:         meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: refresh.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("logout err: %v", err)
	}
	if logout.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("logout result: got=%v", logout.Meta.GetResultCode())
	}

	afterLogoutRefresh, err := svc.RefreshToken(ctx, &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: refresh.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("refresh after logout err: %v", err)
	}
	if afterLogoutRefresh.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied after logout, got=%v", afterLogoutRefresh.Meta.GetResultCode())
	}
}

func TestIdentityRefreshLogoutActorMismatchDenied(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 12, 5, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour)
	ctx := context.Background()

	login, err := svc.Login(ctx, &rgsv1.LoginRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-1", Pin: "1234"},
		},
	})
	if err != nil {
		t.Fatalf("login err: %v", err)
	}
	if login.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected login ok, got=%v", login.Meta.GetResultCode())
	}

	refreshMismatch, err := svc.RefreshToken(ctx, &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: login.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("refresh mismatch err: %v", err)
	}
	if refreshMismatch.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied refresh mismatch, got=%v", refreshMismatch.Meta.GetResultCode())
	}
	if refreshMismatch.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on refresh, got=%q", refreshMismatch.Meta.GetDenialReason())
	}

	logoutMismatch, err := svc.Logout(ctx, &rgsv1.LogoutRequest{
		Meta:         meta("player-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: login.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("logout mismatch err: %v", err)
	}
	if logoutMismatch.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied logout mismatch, got=%v", logoutMismatch.Meta.GetResultCode())
	}
	if logoutMismatch.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on logout, got=%q", logoutMismatch.Meta.GetDenialReason())
	}
}

func TestIdentityOperatorLoginDistinctFromPlayer(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 10, 0, 0, time.UTC)}
	svc := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour)

	operatorLogin, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Credentials: &rgsv1.LoginRequest_Operator{
			Operator: &rgsv1.OperatorCredentials{OperatorId: "op-1", Password: "operator-pass"},
		},
	})
	if err != nil {
		t.Fatalf("operator login err: %v", err)
	}
	if operatorLogin.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("operator login result: got=%v", operatorLogin.Meta.GetResultCode())
	}
	if operatorLogin.Token.GetActor().GetActorType() != rgsv1.ActorType_ACTOR_TYPE_OPERATOR {
		t.Fatalf("expected operator actor type, got=%v", operatorLogin.Token.GetActor().GetActorType())
	}

	denied, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "op-1", Pin: "1234"},
		},
	})
	if err != nil {
		t.Fatalf("mismatched login err: %v", err)
	}
	if denied.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied mismatched actor/credentials, got=%v", denied.Meta.GetResultCode())
	}
}

func TestIdentityLoginLockoutPolicy(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 20, 0, 0, time.UTC)}
	svc := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour)
	svc.maxFailures = 3
	svc.lockoutTTL = 10 * time.Minute

	for i := 0; i < 3; i++ {
		resp, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
			Meta: meta("player-lock-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
			Credentials: &rgsv1.LoginRequest_Player{
				Player: &rgsv1.PlayerCredentials{PlayerId: "player-lock-1", Pin: "bad"},
			},
		})
		if err != nil {
			t.Fatalf("failed login err: %v", err)
		}
		if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
			t.Fatalf("expected denied failed login, got=%v", resp.Meta.GetResultCode())
		}
	}

	locked, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-lock-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-lock-1", Pin: "1234"},
		},
	})
	if err != nil {
		t.Fatalf("locked login err: %v", err)
	}
	if locked.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied while locked, got=%v", locked.Meta.GetResultCode())
	}
	if locked.Meta.GetDenialReason() != "account locked" {
		t.Fatalf("expected account locked reason, got=%q", locked.Meta.GetDenialReason())
	}
}

func TestIdentityLoginRateLimitExceeded(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 25, 0, 0, time.UTC)}
	svc := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour)
	svc.maxFailures = 10
	svc.SetLoginRateLimit(2, time.Minute)

	for i := 0; i < 2; i++ {
		resp, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
			Meta: meta("player-rate-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
			Credentials: &rgsv1.LoginRequest_Player{
				Player: &rgsv1.PlayerCredentials{PlayerId: "player-rate-1", Pin: "bad"},
			},
		})
		if err != nil {
			t.Fatalf("failed login err: %v", err)
		}
		if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
			t.Fatalf("expected denied failed login, got=%v", resp.Meta.GetResultCode())
		}
	}

	limited, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-rate-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-rate-1", Pin: "bad"},
		},
	})
	if err != nil {
		t.Fatalf("rate-limited login err: %v", err)
	}
	if limited.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied while rate-limited, got=%v", limited.Meta.GetResultCode())
	}
	if limited.Meta.GetDenialReason() != "rate limit exceeded" {
		t.Fatalf("expected rate limit exceeded reason, got=%q", limited.Meta.GetDenialReason())
	}
}

func TestResolveActorContextMismatchDenied(t *testing.T) {
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "player-ctx", Type: "ACTOR_TYPE_PLAYER"})
	_, reason := resolveActor(ctx, meta("player-meta", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""))
	if reason != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch, got=%q", reason)
	}
}

func TestIdentitySetCredentialRequiresDatabase(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 30, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "op-1", Type: "ACTOR_TYPE_OPERATOR"})
	resp, err := svc.SetCredential(ctx, &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-new", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "new-pin"),
		Reason:         "bootstrap",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied without database, got=%v", resp.Meta.GetResultCode())
	}
}

func TestIdentitySetCredentialActorMismatchDenied(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 30, 30, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"})
	resp, err := svc.SetCredential(ctx, &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-new", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "new-pin"),
		Reason:         "bootstrap",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied actor mismatch, got=%v", resp.Meta.GetResultCode())
	}
	if resp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch with token reason, got=%q", resp.Meta.GetDenialReason())
	}
}

func TestIdentitySetCredentialRejectsInvalidBcryptHash(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 31, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "op-1", Type: "ACTOR_TYPE_OPERATOR"})
	resp, err := svc.SetCredential(ctx, &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-new", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: "not-a-bcrypt-hash",
		Reason:         "bootstrap",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid for non-bcrypt hash, got=%v", resp.Meta.GetResultCode())
	}
}

func TestIdentitySetCredentialRejectsLowCostBcryptHash(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("new-pin"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate low-cost hash: %v", err)
	}
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 32, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "op-1", Type: "ACTOR_TYPE_OPERATOR"})
	resp, err := svc.SetCredential(ctx, &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-new", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: string(hash),
		Reason:         "bootstrap",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid for low-cost hash, got=%v", resp.Meta.GetResultCode())
	}
}

func TestIdentitySetCredentialAndLoginWithDatabase(t *testing.T) {
	dsn := os.Getenv("RGS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RGS_TEST_DATABASE_URL to run postgres integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`
TRUNCATE TABLE identity_lockouts, identity_credentials RESTART IDENTITY CASCADE
`); err != nil {
		t.Fatalf("truncate identity tables: %v", err)
	}

	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 40, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour, db)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "op-1", Type: "ACTOR_TYPE_OPERATOR"})
	setResp, err := svc.SetCredential(ctx, &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-db-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "player-secret"),
		Reason:         "seed test user",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if setResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected set credential ok, got=%v", setResp.Meta.GetResultCode())
	}

	loginResp, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-db-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-db-1", Pin: "player-secret"},
		},
	})
	if err != nil {
		t.Fatalf("login err: %v", err)
	}
	if loginResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected login ok, got=%v", loginResp.Meta.GetResultCode())
	}

	disableResp, err := svc.DisableCredential(ctx, &rgsv1.DisableCredentialRequest{
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:  &rgsv1.Actor{ActorId: "player-db-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		Reason: "disable test user",
	})
	if err != nil {
		t.Fatalf("disable credential err: %v", err)
	}
	if disableResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected disable ok, got=%v", disableResp.Meta.GetResultCode())
	}
	deniedLogin, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-db-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-db-1", Pin: "player-secret"},
		},
	})
	if err != nil {
		t.Fatalf("login after disable err: %v", err)
	}
	if deniedLogin.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied login when disabled, got=%v", deniedLogin.Meta.GetResultCode())
	}

	enableResp, err := svc.EnableCredential(ctx, &rgsv1.EnableCredentialRequest{
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:  &rgsv1.Actor{ActorId: "player-db-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		Reason: "re-enable test user",
	})
	if err != nil {
		t.Fatalf("enable credential err: %v", err)
	}
	if enableResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected enable ok, got=%v", enableResp.Meta.GetResultCode())
	}
	relogin, err := svc.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-db-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-db-1", Pin: "player-secret"},
		},
	})
	if err != nil {
		t.Fatalf("login after enable err: %v", err)
	}
	if relogin.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected login ok after enable, got=%v", relogin.Meta.GetResultCode())
	}
}

func mustBcryptHash(t *testing.T, secret string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("generate bcrypt hash: %v", err)
	}
	return string(hash)
}

func TestIdentityGetAndResetLockout(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 13, 50, 0, 0, time.UTC)}
	svc := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour)
	svc.maxFailures = 2
	svc.lockoutTTL = 5 * time.Minute

	for i := 0; i < 2; i++ {
		_, _ = svc.Login(context.Background(), &rgsv1.LoginRequest{
			Meta: meta("player-lock-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
			Credentials: &rgsv1.LoginRequest_Player{
				Player: &rgsv1.PlayerCredentials{PlayerId: "player-lock-2", Pin: "wrong"},
			},
		})
	}

	adminCtx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "op-1", Type: "ACTOR_TYPE_OPERATOR"})
	statusResp, err := svc.GetLockout(adminCtx, &rgsv1.GetLockoutRequest{
		Meta:  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor: &rgsv1.Actor{ActorId: "player-lock-2", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
	})
	if err != nil {
		t.Fatalf("get lockout err: %v", err)
	}
	if statusResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected get lockout ok, got=%v", statusResp.Meta.GetResultCode())
	}
	if !statusResp.Status.GetLocked() {
		t.Fatalf("expected locked=true after repeated failures")
	}

	resetResp, err := svc.ResetLockout(adminCtx, &rgsv1.ResetLockoutRequest{
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:  &rgsv1.Actor{ActorId: "player-lock-2", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		Reason: "operator unlock",
	})
	if err != nil {
		t.Fatalf("reset lockout err: %v", err)
	}
	if resetResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected reset lockout ok, got=%v", resetResp.Meta.GetResultCode())
	}
	if resetResp.Status.GetLocked() {
		t.Fatalf("expected locked=false after reset")
	}
}
