package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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
