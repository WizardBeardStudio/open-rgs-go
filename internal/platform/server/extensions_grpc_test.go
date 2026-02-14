package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func assertMetaFields(t *testing.T, m *rgsv1.ResponseMeta, wantRequestID string) {
	t.Helper()
	if m == nil {
		t.Fatalf("expected response meta, got nil")
	}
	if m.GetRequestId() != wantRequestID {
		t.Fatalf("expected request_id %q, got=%q", wantRequestID, m.GetRequestId())
	}
	if m.GetServerTime() == "" {
		t.Fatalf("expected non-empty server_time")
	}
}

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

func TestPromotionsRecordBonusTransactionNilRequest(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 9, 55, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordBonusTransaction(ctx, nil)
	if err != nil {
		t.Fatalf("record bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for nil request, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "transaction requires equipment_id, player_id, and positive amount" {
		t.Fatalf("expected invalid-request denial reason, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_bonus_transaction" || events[len(events)-1].Reason != "invalid request" {
		t.Fatalf("expected invalid-request audit event, got=%v", events)
	}
}

func TestPromotionsListRecentNilRequestDeniedActorRequired(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 9, 56, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListRecentBonusTransactions(ctx, nil)
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for nil request, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "actor is required" {
		t.Fatalf("expected denial reason actor is required, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_recent_bonus_transactions" || events[len(events)-1].Reason != "actor is required" {
		t.Fatalf("expected denied audit event for missing actor, got=%v", events)
	}
}

func TestPromotionsListRecentRejectsNegativeLimit(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 2, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListRecentBonusTransactions(ctx, &rgsv1.ListRecentBonusTransactionsRequest{
		Meta:  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Limit: -1,
	})
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for negative limit, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid limit" {
		t.Fatalf("expected denial reason invalid limit, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "req-1")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_recent_bonus_transactions" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid bonus list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid limit" {
		t.Fatalf("expected audit reason invalid limit, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListRecentRejectsOversizedLimit(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 3, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListRecentBonusTransactions(ctx, &rgsv1.ListRecentBonusTransactionsRequest{
		Meta:  meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Limit: 101,
	})
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for oversized limit, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid limit" {
		t.Fatalf("expected denial reason invalid limit, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_recent_bonus_transactions" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for oversized bonus list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid limit" {
		t.Fatalf("expected audit reason invalid limit, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListRecentDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 4, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListRecentBonusTransactions(ctx, &rgsv1.ListRecentBonusTransactionsRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
	})
	if err != nil {
		t.Fatalf("list bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "req-1")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_recent_bonus_transactions" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for bonus list access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsRecordBonusTransactionRejectsInvalidOccurredAt(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 5, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordBonusTransaction(ctx, &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
			OccurredAt:  "bad-time",
		},
	})
	if err != nil {
		t.Fatalf("record bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for malformed occurred_at, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid occurred_at" {
		t.Fatalf("expected denial reason invalid occurred_at, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_bonus_transaction" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid bonus request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid occurred_at" {
		t.Fatalf("expected audit reason invalid occurred_at, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsRecordBonusTransactionDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 6, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordBonusTransaction(ctx, &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("record bonus tx err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "req-1")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_bonus_transaction" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for bonus write access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
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

func TestUISystemOverlaySubmitNilRequest(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 56, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.SubmitSystemWindowEvent(ctx, nil)
	if err != nil {
		t.Fatalf("submit window event err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for nil request, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "event requires equipment_id, window_id, and event_type" {
		t.Fatalf("expected invalid-request denial reason, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "submit_system_window_event" || events[len(events)-1].Reason != "invalid request" {
		t.Fatalf("expected invalid-request audit event, got=%v", events)
	}
}

func TestUISystemOverlayListNilRequestDeniedActorRequired(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 10, 57, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, nil)
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for nil request, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "actor is required" {
		t.Fatalf("expected denial reason actor is required, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Reason != "actor is required" {
		t.Fatalf("expected denied audit event for missing actor, got=%v", events)
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

func TestPromotionsListPromotionalAwardsPagination(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := svc.RecordPromotionalAward(ctx, &rgsv1.RecordPromotionalAwardRequest{
			Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
			Award: &rgsv1.PromotionalAward{
				PlayerId:   "player-1",
				CampaignId: "camp-1",
				AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
				Amount:     &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
				OccurredAt: clk.now.Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			t.Fatalf("record promotional award err: %v", err)
		}
	}

	first, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PlayerId:   "player-1",
		CampaignId: "camp-1",
		PageSize:   2,
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if len(first.Awards) != 2 {
		t.Fatalf("expected first page size 2, got=%d", len(first.Awards))
	}
	if first.NextPageToken == "" {
		t.Fatalf("expected non-empty next page token")
	}

	second, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PlayerId:   "player-1",
		CampaignId: "camp-1",
		PageSize:   2,
		PageToken:  first.NextPageToken,
	})
	if err != nil {
		t.Fatalf("list awards page 2 err: %v", err)
	}
	if len(second.Awards) != 1 {
		t.Fatalf("expected second page size 1, got=%d", len(second.Awards))
	}
}

func TestPromotionsDisableInMemoryCacheSkipsAwardMirror(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 30, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	svc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	_, err := svc.RecordPromotionalAward(ctx, &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("record promotional award err: %v", err)
	}

	list, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if len(list.Awards) != 0 {
		t.Fatalf("expected no in-memory awards when cache disabled, got=%d", len(list.Awards))
	}
}

func TestPromotionsListAwardsRejectsNegativePageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 35, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: -1,
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for negative page_size, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected denial reason invalid page_size, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_promotional_awards" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid awards list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_size" {
		t.Fatalf("expected audit reason invalid page_size, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListAwardsRejectsOversizedPageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 36, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: 101,
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for oversized page_size, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected denial reason invalid page_size, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_promotional_awards" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for oversized awards list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_size" {
		t.Fatalf("expected audit reason invalid page_size, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListAwardsDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 39, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_promotional_awards" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for awards list access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsRecordPromotionalAwardRejectsUnknownAwardType(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 45, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordPromotionalAward(ctx, &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType(99),
			Amount:     &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("record promotional award err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for unknown award type, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "award requires player_id, award_type, and positive amount" {
		t.Fatalf("expected denial reason for invalid award request, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_promotional_award" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid award request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid request" {
		t.Fatalf("expected audit reason invalid request, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsRecordPromotionalAwardRejectsInvalidOccurredAt(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 46, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordPromotionalAward(ctx, &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
			OccurredAt: "bad-time",
		},
	})
	if err != nil {
		t.Fatalf("record promotional award err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for malformed occurred_at, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid occurred_at" {
		t.Fatalf("expected denial reason invalid occurred_at, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_promotional_award" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid award timestamp, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid occurred_at" {
		t.Fatalf("expected audit reason invalid occurred_at, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsRecordPromotionalAwardDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 47, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.RecordPromotionalAward(ctx, &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("record promotional award err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "record_promotional_award" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for award write access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
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

func TestUISystemOverlaySubmitRejectsUnknownEventType(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 40, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.SubmitSystemWindowEvent(ctx, &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType(99),
		},
	})
	if err != nil {
		t.Fatalf("submit window event err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for unknown event type, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "event requires equipment_id, window_id, and event_type" {
		t.Fatalf("expected denial reason for invalid ui event request, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "submit_system_window_event" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid ui event type, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid request" {
		t.Fatalf("expected audit reason invalid request, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlaySubmitRejectsInvalidEventTime(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 41, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.SubmitSystemWindowEvent(ctx, &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
			EventTime:   "bad-time",
		},
	})
	if err != nil {
		t.Fatalf("submit window event err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for malformed event_time, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid event_time" {
		t.Fatalf("expected denial reason invalid event_time, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "submit_system_window_event" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid ui submit request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid event_time" {
		t.Fatalf("expected audit reason invalid event_time, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlaySubmitDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 44, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.SubmitSystemWindowEvent(ctx, &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
		},
	})
	if err != nil {
		t.Fatalf("submit window event err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "submit_system_window_event" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for ui submit access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListRejectsNegativePageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 42, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: -1,
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for negative page_size, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected denial reason invalid page_size, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for negative ui page_size, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_size" {
		t.Fatalf("expected audit reason invalid page_size, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListRejectsOversizedPageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 43, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: 201,
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for oversized page_size, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected denial reason invalid page_size, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for oversized ui page_size, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_size" {
		t.Fatalf("expected audit reason invalid page_size, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListDeniedForPlayerActor(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 45, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for player actor, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected denial reason unauthorized actor type, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for ui list access, got=%v", events)
	}
	if events[len(events)-1].Reason != "unauthorized actor type" {
		t.Fatalf("expected audit reason unauthorized actor type, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListRejectsInvalidPageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 45, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "bad-token",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for bad page token, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected denial reason invalid page_token, got=%q", resp.GetMeta().GetDenialReason())
	}
	assertMetaFields(t, resp.GetMeta(), "req-1")
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid ui list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_token" {
		t.Fatalf("expected audit reason invalid page_token, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListRejectsNegativePageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 56, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "-1",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for negative page token, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected denial reason invalid page_token, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for negative page token, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_token" {
		t.Fatalf("expected audit reason invalid page_token, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListAwardsRejectsNegativePageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 37, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "-1",
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for negative page token, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected denial reason invalid page_token, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_promotional_awards" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for negative page token, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_token" {
		t.Fatalf("expected audit reason invalid page_token, got=%q", events[len(events)-1].Reason)
	}
}

func TestPromotionsListAwardsRejectsInvalidPageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 38, 0, 0, time.UTC)}
	svc := NewPromotionsService(clk)
	ctx := context.Background()

	resp, err := svc.ListPromotionalAwards(ctx, &rgsv1.ListPromotionalAwardsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "bad-token",
	})
	if err != nil {
		t.Fatalf("list awards err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for malformed page token, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected denial reason invalid page_token, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_promotional_awards" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for invalid awards list request, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid page_token" {
		t.Fatalf("expected audit reason invalid page_token, got=%q", events[len(events)-1].Reason)
	}
}

func TestUISystemOverlayListRejectsInvalidTimeInputs(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 50, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		FromTime: "not-a-time",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for bad from_time, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid from_time" {
		t.Fatalf("expected denial reason invalid from_time, got=%q", resp.GetMeta().GetDenialReason())
	}

	resp, err = svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:   meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ToTime: "not-a-time",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for bad to_time, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "invalid to_time" {
		t.Fatalf("expected denial reason invalid to_time, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) < 2 {
		t.Fatalf("expected denied audit events for invalid time inputs, got=%v", events)
	}
	last := events[len(events)-1]
	prev := events[len(events)-2]
	if prev.Action != "list_system_window_events" || prev.Result != "denied" || last.Action != "list_system_window_events" || last.Result != "denied" {
		t.Fatalf("expected denied audit events for invalid time inputs, got prev=%v last=%v", prev, last)
	}
	if prev.Reason != "invalid from_time" || last.Reason != "invalid to_time" {
		t.Fatalf("expected invalid time reasons from/to, got prev=%q last=%q", prev.Reason, last.Reason)
	}
}

func TestUISystemOverlayListRejectsInvertedTimeRange(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 11, 55, 0, 0, time.UTC)}
	svc := NewUISystemOverlayService(clk)
	ctx := context.Background()

	resp, err := svc.ListSystemWindowEvents(ctx, &rgsv1.ListSystemWindowEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		FromTime: "2026-02-16T12:00:00Z",
		ToTime:   "2026-02-16T11:00:00Z",
	})
	if err != nil {
		t.Fatalf("list window events err: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid result for inverted range, got=%s", resp.GetMeta().GetResultCode().String())
	}
	if resp.GetMeta().GetDenialReason() != "from_time must be <= to_time" {
		t.Fatalf("expected denial reason from_time must be <= to_time, got=%q", resp.GetMeta().GetDenialReason())
	}
	events := svc.AuditStore.Events()
	if len(events) == 0 || events[len(events)-1].Action != "list_system_window_events" || events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit event for inverted range, got=%v", events)
	}
	if events[len(events)-1].Reason != "invalid time range" {
		t.Fatalf("expected audit reason invalid time range, got=%q", events[len(events)-1].Reason)
	}
}
