package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
)

func TestSessionsStartGetEndWorkflow(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 10, 0, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)
	ctx := context.Background()

	start, err := svc.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}
	if start.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("start session code=%v reason=%q", start.Meta.GetResultCode(), start.Meta.GetDenialReason())
	}
	if start.Session.GetSessionId() == "" {
		t.Fatalf("expected session id")
	}

	got, err := svc.GetSession(ctx, &rgsv1.GetSessionRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: start.Session.GetSessionId(),
	})
	if err != nil {
		t.Fatalf("get session err: %v", err)
	}
	if got.Session.GetPlayerId() != "player-1" || got.Session.GetState() != rgsv1.SessionState_SESSION_STATE_ACTIVE {
		t.Fatalf("unexpected session: %+v", got.Session)
	}

	ended, err := svc.EndSession(ctx, &rgsv1.EndSessionRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: start.Session.GetSessionId(),
		Reason:    "player logout",
	})
	if err != nil {
		t.Fatalf("end session err: %v", err)
	}
	if ended.Session.GetState() != rgsv1.SessionState_SESSION_STATE_ENDED {
		t.Fatalf("expected ended session, got=%v", ended.Session.GetState())
	}
	if ended.Session.GetEndedAt() == "" {
		t.Fatalf("expected ended_at")
	}
}

func TestSessionsStartDeniedForMismatchedPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 10, 10, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)
	ctx := context.Background()

	resp, err := svc.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:     meta("player-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied, got=%v", resp.Meta.GetResultCode())
	}
}

func TestSessionsGetAutoExpiresOnTimeout(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 11, 0, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)
	ctx := context.Background()

	start, err := svc.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:                  meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId:              "player-1",
		DeviceId:              "device-a",
		SessionTimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}

	svc.Clock = ledgerFixedClock{now: clk.now.Add(2 * time.Second)}
	got, err := svc.GetSession(ctx, &rgsv1.GetSessionRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: start.Session.GetSessionId(),
	})
	if err != nil {
		t.Fatalf("get session err: %v", err)
	}
	if got.Session.GetState() != rgsv1.SessionState_SESSION_STATE_EXPIRED {
		t.Fatalf("expected expired state, got=%v", got.Session.GetState())
	}
	if got.Session.GetEndedAt() == "" {
		t.Fatalf("expected ended_at when expired")
	}
}

func TestSessionsDisableInMemoryCacheRequiresPersistence(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 11, 20, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	resp, err := svc.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_ERROR {
		t.Fatalf("expected persistence error, got=%v", resp.Meta.GetResultCode())
	}
}

func TestSessionsActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 11, 30, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-player", Type: "ACTOR_TYPE_PLAYER"})

	start, err := svc.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}
	if start.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied start, got=%v", start.Meta.GetResultCode())
	}
	if start.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on start, got=%q", start.Meta.GetDenialReason())
	}

	seed, err := svc.StartSession(context.Background(), &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("seed start session err: %v", err)
	}

	got, err := svc.GetSession(ctx, &rgsv1.GetSessionRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: seed.Session.GetSessionId(),
	})
	if err != nil {
		t.Fatalf("get session err: %v", err)
	}
	if got.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied get, got=%v", got.Meta.GetResultCode())
	}
	if got.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on get, got=%q", got.Meta.GetDenialReason())
	}

	ended, err := svc.EndSession(ctx, &rgsv1.EndSessionRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: seed.Session.GetSessionId(),
		Reason:    "logout",
	})
	if err != nil {
		t.Fatalf("end session err: %v", err)
	}
	if ended.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied end, got=%v", ended.Meta.GetResultCode())
	}
	if ended.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on end, got=%q", ended.Meta.GetDenialReason())
	}

	events := svc.AuditStore.Events()
	if len(events) < 3 {
		t.Fatalf("expected denied session audit events, got=%v", events)
	}
	last := events[len(events)-1]
	if last.Action != "end_session" || last.Reason != "actor mismatch with token" {
		t.Fatalf("expected denied end_session audit reason actor mismatch with token, got=%+v", last)
	}
}
