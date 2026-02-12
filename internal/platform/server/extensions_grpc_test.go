package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func TestPromotionsListRecentDefaultsTo25(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 0, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	for i := 0; i < 30; i++ {
		_, err := svc.RecordBonusTransaction(ctx, &rgsv1.RecordBonusTransactionRequest{
			Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
			Transaction: &rgsv1.BonusTransaction{
				EquipmentId: "eq-1",
				PlayerId:    "player-1",
				Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
				OccurredAt:  clk.now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			t.Fatalf("record bonus tx err: %v", err)
		}
	}

	list, err := svc.ListRecentBonusTransactions(ctx, &rgsv1.ListRecentBonusTransactionsRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-1",
	})
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if len(list.Transactions) != 25 {
		t.Fatalf("expected default limit of 25, got=%d", len(list.Transactions))
	}
}

func TestUISystemOverlaySubmitAndListPagination(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 0, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.SubmitSystemWindowEvent(ctx, &rgsv1.SubmitSystemWindowEventRequest{
			Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
			Event: &rgsv1.SystemWindowEvent{
				EquipmentId: "eq-1",
				PlayerId:    "player-1",
				WindowId:    "sys-menu",
				EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
				EventTime:   clk.now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			t.Fatalf("submit window event err: %v", err)
		}
	}

	first, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-1",
		PageSize:    2,
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if len(first.Events) != 2 {
		t.Fatalf("expected first page size 2, got=%d", len(first.Events))
	}
	if first.NextPageToken == "" {
		t.Fatalf("expected non-empty next page token")
	}

	second, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-1",
		PageSize:    2,
		PageToken:   first.NextPageToken,
	})
	if err != nil {
		t.Fatalf("list window events page 2 err: %v", err)
	}
	if len(second.Events) != 1 {
		t.Fatalf("expected second page size 1, got=%d", len(second.Events))
	}
}

func TestPromotionsDisableInMemoryCacheSkipsBonusMirror(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 30, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	_, err := svc.RecordBonusTransaction(ctx, &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("record bonus tx err: %v", err)
	}

	list, err := svc.ListRecentBonusTransactions(ctx, &rgsv1.ListRecentBonusTransactionsRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-1",
	})
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if len(list.Transactions) != 0 {
		t.Fatalf("expected no in-memory bonus transactions when cache disabled, got=%d", len(list.Transactions))
	}
}

func TestUISystemOverlayDisableInMemoryCacheSkipsEventMirror(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 30, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	_, err := svc.SubmitSystemWindowEvent(ctx, &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
			EventTime:   clk.now.Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		t.Fatalf("submit window event err: %v", err)
	}

	list, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		EquipmentId: "eq-1",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if len(list.Events) != 0 {
		t.Fatalf("expected no in-memory window events when cache disabled, got=%d", len(list.Events))
	}
}
