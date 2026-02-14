package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
)

func TestWageringPlaceSettleIdempotent(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC)}
	svc := NewWageringService(clk)
	ctx := context.Background()

	placeReq := &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-place-1"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	}
	firstPlace, err := svc.PlaceWager(ctx, placeReq)
	if err != nil {
		t.Fatalf("place wager err: %v", err)
	}
	secondPlace, err := svc.PlaceWager(ctx, placeReq)
	if err != nil {
		t.Fatalf("place wager replay err: %v", err)
	}
	if firstPlace.Wager.GetWagerId() != secondPlace.Wager.GetWagerId() {
		t.Fatalf("expected idempotent place wager id, got first=%s second=%s", firstPlace.Wager.GetWagerId(), secondPlace.Wager.GetWagerId())
	}

	settleReq := &rgsv1.SettleWagerRequest{
		Meta:       meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, "idem-wager-settle-1"),
		WagerId:    firstPlace.Wager.GetWagerId(),
		Payout:     &rgsv1.Money{AmountMinor: 400, Currency: "USD"},
		OutcomeRef: "outcome-123",
	}
	firstSettle, err := svc.SettleWager(ctx, settleReq)
	if err != nil {
		t.Fatalf("settle wager err: %v", err)
	}
	secondSettle, err := svc.SettleWager(ctx, settleReq)
	if err != nil {
		t.Fatalf("settle wager replay err: %v", err)
	}
	if firstSettle.Wager.GetStatus() != rgsv1.WagerStatus_WAGER_STATUS_SETTLED {
		t.Fatalf("expected settled status, got=%v", firstSettle.Wager.GetStatus())
	}
	if firstSettle.Wager.GetWagerId() != secondSettle.Wager.GetWagerId() {
		t.Fatalf("expected idempotent settle wager id")
	}
}

func TestWageringCancelDeniedForPlayer(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 15, 10, 5, 0, 0, time.UTC)}
	svc := NewWageringService(clk)

	place, err := svc.PlaceWager(context.Background(), &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-place-2"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("place wager err: %v", err)
	}

	cancel, err := svc.CancelWager(context.Background(), &rgsv1.CancelWagerRequest{
		Meta:    meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-cancel-1"),
		WagerId: place.Wager.GetWagerId(),
		Reason:  "player cancel attempt",
	})
	if err != nil {
		t.Fatalf("cancel wager err: %v", err)
	}
	if cancel.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied cancel for player actor, got=%v", cancel.Meta.GetResultCode())
	}
}

func TestWageringDisableInMemoryIdempotencyKeepsNonDBStateMirror(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 15, 10, 10, 0, 0, time.UTC)}
	svc := NewWageringService(clk)
	svc.SetDisableInMemoryIdempotencyCache(true)

	place, err := svc.PlaceWager(context.Background(), &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-place-3"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("place wager err: %v", err)
	}

	settle, err := svc.SettleWager(context.Background(), &rgsv1.SettleWagerRequest{
		Meta:       meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, "idem-wager-settle-3"),
		WagerId:    place.Wager.GetWagerId(),
		Payout:     &rgsv1.Money{AmountMinor: 150, Currency: "USD"},
		OutcomeRef: "outcome-xyz",
	})
	if err != nil {
		t.Fatalf("settle wager err: %v", err)
	}
	if settle.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected settle ok, got=%v", settle.Meta.GetResultCode())
	}
}

func TestWageringActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 15, 10, 15, 0, 0, time.UTC)}
	svc := NewWageringService(clk)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-player", Type: "ACTOR_TYPE_PLAYER"})

	place, err := svc.PlaceWager(ctx, &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-place-mismatch"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("place wager err: %v", err)
	}
	if place.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied place for actor mismatch, got=%v", place.Meta.GetResultCode())
	}
	if place.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on place, got=%q", place.Meta.GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "place_wager" || events[len(events)-1].Reason != "actor mismatch with token" {
		t.Fatalf("expected denied place audit for actor mismatch, got=%v", events)
	}

	seed, err := svc.PlaceWager(context.Background(), &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-wager-place-seed"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("seed place wager err: %v", err)
	}
	settle, err := svc.SettleWager(ctx, &rgsv1.SettleWagerRequest{
		Meta:       meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, "idem-wager-settle-mismatch"),
		WagerId:    seed.Wager.GetWagerId(),
		Payout:     &rgsv1.Money{AmountMinor: 120, Currency: "USD"},
		OutcomeRef: "outcome-1",
	})
	if err != nil {
		t.Fatalf("settle wager err: %v", err)
	}
	if settle.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied settle for actor mismatch, got=%v", settle.Meta.GetResultCode())
	}
	if settle.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on settle, got=%q", settle.Meta.GetDenialReason())
	}

	cancel, err := svc.CancelWager(ctx, &rgsv1.CancelWagerRequest{
		Meta:    meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "idem-wager-cancel-mismatch"),
		WagerId: seed.Wager.GetWagerId(),
		Reason:  "ops cancel",
	})
	if err != nil {
		t.Fatalf("cancel wager err: %v", err)
	}
	if cancel.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied cancel for actor mismatch, got=%v", cancel.Meta.GetResultCode())
	}
	if cancel.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial on cancel, got=%q", cancel.Meta.GetDenialReason())
	}
	events = svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "cancel_wager" || events[len(events)-1].Reason != "actor mismatch with token" {
		t.Fatalf("expected denied cancel audit for actor mismatch, got=%v", events)
	}
}
