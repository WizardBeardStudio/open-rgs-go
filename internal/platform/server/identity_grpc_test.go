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
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:  &rgsv1.Actor{ActorId: "player-new", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		Secret: "new-pin",
		Reason: "bootstrap",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied without database, got=%v", resp.Meta.GetResultCode())
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
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:  &rgsv1.Actor{ActorId: "player-db-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		Secret: "player-secret",
		Reason: "seed test user",
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
}
