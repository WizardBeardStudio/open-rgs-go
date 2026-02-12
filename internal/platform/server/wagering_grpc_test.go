package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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
