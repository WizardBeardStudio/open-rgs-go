package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestExtensionsGatewayParity_Workflow(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)}
	promoSvc := NewPromotionsService(clk)
	uiSvc := NewUISystemOverlayService(clk)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterPromotionsServiceHandlerServer(context.Background(), gwMux, promoSvc); err != nil {
		t.Fatalf("register promotions gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterUISystemOverlayServiceHandlerServer(context.Background(), gwMux, uiSvc); err != nil {
		t.Fatalf("register ui gateway handlers: %v", err)
	}

	bonusReq := &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	}
	bonusBody, _ := protojson.Marshal(bonusReq)
	bonusHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/bonus-transactions", bytes.NewReader(bonusBody))
	bonusHTTPReq.Header.Set("Content-Type", "application/json")
	bonusRec := httptest.NewRecorder()
	gwMux.ServeHTTP(bonusRec, bonusHTTPReq)
	if bonusRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("bonus record status: got=%d body=%s", bonusRec.Result().StatusCode, bonusRec.Body.String())
	}

	qBonus := make(url.Values)
	qBonus.Set("meta.actor.actorId", "op-1")
	qBonus.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBonus.Set("equipment_id", "eq-1")
	listBonusReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/bonus-transactions?"+qBonus.Encode(), nil)
	listBonusRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listBonusRec, listBonusReq)
	if listBonusRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("bonus list status: got=%d body=%s", listBonusRec.Result().StatusCode, listBonusRec.Body.String())
	}
	var listBonusResp rgsv1.ListRecentBonusTransactionsResponse
	if err := protojson.Unmarshal(listBonusRec.Body.Bytes(), &listBonusResp); err != nil {
		t.Fatalf("unmarshal bonus list response: %v", err)
	}
	if len(listBonusResp.Transactions) != 1 {
		t.Fatalf("expected 1 bonus transaction, got=%d", len(listBonusResp.Transactions))
	}

	awardReq := &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 50, Currency: "USD"},
			OccurredAt: clk.now.Format(time.RFC3339Nano),
		},
	}
	awardBody, _ := protojson.Marshal(awardReq)
	awardHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/awards", bytes.NewReader(awardBody))
	awardHTTPReq.Header.Set("Content-Type", "application/json")
	awardRec := httptest.NewRecorder()
	gwMux.ServeHTTP(awardRec, awardHTTPReq)
	if awardRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("award record status: got=%d body=%s", awardRec.Result().StatusCode, awardRec.Body.String())
	}

	qAwards := make(url.Values)
	qAwards.Set("meta.actor.actorId", "op-1")
	qAwards.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qAwards.Set("player_id", "player-1")
	listAwardsReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qAwards.Encode(), nil)
	listAwardsRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listAwardsRec, listAwardsReq)
	if listAwardsRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("award list status: got=%d body=%s", listAwardsRec.Result().StatusCode, listAwardsRec.Body.String())
	}
	var listAwardsResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(listAwardsRec.Body.Bytes(), &listAwardsResp); err != nil {
		t.Fatalf("unmarshal award list response: %v", err)
	}
	if len(listAwardsResp.Awards) != 1 {
		t.Fatalf("expected 1 promotional award, got=%d", len(listAwardsResp.Awards))
	}

	uiReq := &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
			EventTime:   clk.now.Format(time.RFC3339Nano),
		},
	}
	uiBody, _ := protojson.Marshal(uiReq)
	uiHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/ui/system-window-events", bytes.NewReader(uiBody))
	uiHTTPReq.Header.Set("Content-Type", "application/json")
	uiRec := httptest.NewRecorder()
	gwMux.ServeHTTP(uiRec, uiHTTPReq)
	if uiRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui submit status: got=%d body=%s", uiRec.Result().StatusCode, uiRec.Body.String())
	}

	qUI := make(url.Values)
	qUI.Set("meta.actor.actorId", "op-1")
	qUI.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qUI.Set("equipment_id", "eq-1")
	listUIReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qUI.Encode(), nil)
	listUIRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listUIRec, listUIReq)
	if listUIRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list status: got=%d body=%s", listUIRec.Result().StatusCode, listUIRec.Body.String())
	}
	var listUIResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(listUIRec.Body.Bytes(), &listUIResp); err != nil {
		t.Fatalf("unmarshal ui list response: %v", err)
	}
	if len(listUIResp.Events) != 1 {
		t.Fatalf("expected 1 system window event, got=%d", len(listUIResp.Events))
	}
}

func TestExtensionsGatewayParity_ValidationErrors(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)}
	promoSvc := NewPromotionsService(clk)
	uiSvc := NewUISystemOverlayService(clk)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterPromotionsServiceHandlerServer(context.Background(), gwMux, promoSvc); err != nil {
		t.Fatalf("register promotions gateway handlers: %v", err)
	}
	if err := rgsv1.RegisterUISystemOverlayServiceHandlerServer(context.Background(), gwMux, uiSvc); err != nil {
		t.Fatalf("register ui gateway handlers: %v", err)
	}

	awardReq := &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType(99),
			Amount:     &rgsv1.Money{AmountMinor: 50, Currency: "USD"},
		},
	}
	awardBody, _ := protojson.Marshal(awardReq)
	awardHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/awards", bytes.NewReader(awardBody))
	awardHTTPReq.Header.Set("Content-Type", "application/json")
	awardRec := httptest.NewRecorder()
	gwMux.ServeHTTP(awardRec, awardHTTPReq)
	if awardRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("award record status: got=%d body=%s", awardRec.Result().StatusCode, awardRec.Body.String())
	}
	var awardResp rgsv1.RecordPromotionalAwardResponse
	if err := protojson.Unmarshal(awardRec.Body.Bytes(), &awardResp); err != nil {
		t.Fatalf("unmarshal award response: %v", err)
	}
	if awardResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid award result code, got=%s", awardResp.GetMeta().GetResultCode().String())
	}

	bonusReq := &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
			OccurredAt:  "bad-time",
		},
	}
	bonusBody, _ := protojson.Marshal(bonusReq)
	bonusHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/bonus-transactions", bytes.NewReader(bonusBody))
	bonusHTTPReq.Header.Set("Content-Type", "application/json")
	bonusRec := httptest.NewRecorder()
	gwMux.ServeHTTP(bonusRec, bonusHTTPReq)
	if bonusRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("bonus record status: got=%d body=%s", bonusRec.Result().StatusCode, bonusRec.Body.String())
	}
	var bonusResp rgsv1.RecordBonusTransactionResponse
	if err := protojson.Unmarshal(bonusRec.Body.Bytes(), &bonusResp); err != nil {
		t.Fatalf("unmarshal bonus response: %v", err)
	}
	if bonusResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid bonus result code, got=%s", bonusResp.GetMeta().GetResultCode().String())
	}

	awardBadTimeReq := &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 50, Currency: "USD"},
			OccurredAt: "bad-time",
		},
	}
	awardBadTimeBody, _ := protojson.Marshal(awardBadTimeReq)
	awardBadTimeHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/awards", bytes.NewReader(awardBadTimeBody))
	awardBadTimeHTTPReq.Header.Set("Content-Type", "application/json")
	awardBadTimeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(awardBadTimeRec, awardBadTimeHTTPReq)
	if awardBadTimeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("award record bad time status: got=%d body=%s", awardBadTimeRec.Result().StatusCode, awardBadTimeRec.Body.String())
	}
	var awardBadTimeResp rgsv1.RecordPromotionalAwardResponse
	if err := protojson.Unmarshal(awardBadTimeRec.Body.Bytes(), &awardBadTimeResp); err != nil {
		t.Fatalf("unmarshal award bad time response: %v", err)
	}
	if awardBadTimeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid award bad-time result code, got=%s", awardBadTimeResp.GetMeta().GetResultCode().String())
	}

	uiReq := &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
			EventTime:   "bad-time",
		},
	}
	uiBody, _ := protojson.Marshal(uiReq)
	uiHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/ui/system-window-events", bytes.NewReader(uiBody))
	uiHTTPReq.Header.Set("Content-Type", "application/json")
	uiRec := httptest.NewRecorder()
	gwMux.ServeHTTP(uiRec, uiHTTPReq)
	if uiRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui submit status: got=%d body=%s", uiRec.Result().StatusCode, uiRec.Body.String())
	}
	var uiResp rgsv1.SubmitSystemWindowEventResponse
	if err := protojson.Unmarshal(uiRec.Body.Bytes(), &uiResp); err != nil {
		t.Fatalf("unmarshal ui submit response: %v", err)
	}
	if uiResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid ui submit result code, got=%s", uiResp.GetMeta().GetResultCode().String())
	}

	qBadPageToken := make(url.Values)
	qBadPageToken.Set("meta.actor.actorId", "op-1")
	qBadPageToken.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadPageToken.Set("page_token", "bad-token")
	badPageReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qBadPageToken.Encode(), nil)
	badPageRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badPageRec, badPageReq)
	if badPageRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list bad page token status: got=%d body=%s", badPageRec.Result().StatusCode, badPageRec.Body.String())
	}
	var badPageResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(badPageRec.Body.Bytes(), &badPageResp); err != nil {
		t.Fatalf("unmarshal ui list bad page token response: %v", err)
	}
	if badPageResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid page token result code, got=%s", badPageResp.GetMeta().GetResultCode().String())
	}

	qBadRange := make(url.Values)
	qBadRange.Set("meta.actor.actorId", "op-1")
	qBadRange.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadRange.Set("from_time", "2026-02-16T13:00:00Z")
	qBadRange.Set("to_time", "2026-02-16T12:00:00Z")
	badRangeReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qBadRange.Encode(), nil)
	badRangeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badRangeRec, badRangeReq)
	if badRangeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list bad range status: got=%d body=%s", badRangeRec.Result().StatusCode, badRangeRec.Body.String())
	}
	var badRangeResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(badRangeRec.Body.Bytes(), &badRangeResp); err != nil {
		t.Fatalf("unmarshal ui list bad range response: %v", err)
	}
	if badRangeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid bad range result code, got=%s", badRangeResp.GetMeta().GetResultCode().String())
	}

	qBadBonusLimit := make(url.Values)
	qBadBonusLimit.Set("meta.actor.actorId", "op-1")
	qBadBonusLimit.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadBonusLimit.Set("limit", "-1")
	badBonusLimitReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/bonus-transactions?"+qBadBonusLimit.Encode(), nil)
	badBonusLimitRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badBonusLimitRec, badBonusLimitReq)
	if badBonusLimitRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("bonus list bad limit status: got=%d body=%s", badBonusLimitRec.Result().StatusCode, badBonusLimitRec.Body.String())
	}
	var badBonusLimitResp rgsv1.ListRecentBonusTransactionsResponse
	if err := protojson.Unmarshal(badBonusLimitRec.Body.Bytes(), &badBonusLimitResp); err != nil {
		t.Fatalf("unmarshal bonus list bad limit response: %v", err)
	}
	if badBonusLimitResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid bonus limit result code, got=%s", badBonusLimitResp.GetMeta().GetResultCode().String())
	}

	qBadAwardPageSize := make(url.Values)
	qBadAwardPageSize.Set("meta.actor.actorId", "op-1")
	qBadAwardPageSize.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadAwardPageSize.Set("page_size", "-1")
	badAwardPageSizeReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qBadAwardPageSize.Encode(), nil)
	badAwardPageSizeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badAwardPageSizeRec, badAwardPageSizeReq)
	if badAwardPageSizeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("awards list bad page_size status: got=%d body=%s", badAwardPageSizeRec.Result().StatusCode, badAwardPageSizeRec.Body.String())
	}
	var badAwardPageSizeResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(badAwardPageSizeRec.Body.Bytes(), &badAwardPageSizeResp); err != nil {
		t.Fatalf("unmarshal awards list bad page_size response: %v", err)
	}
	if badAwardPageSizeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid awards page_size result code, got=%s", badAwardPageSizeResp.GetMeta().GetResultCode().String())
	}

	qBadAwardPageToken := make(url.Values)
	qBadAwardPageToken.Set("meta.actor.actorId", "op-1")
	qBadAwardPageToken.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadAwardPageToken.Set("page_token", "bad-token")
	badAwardPageTokenReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qBadAwardPageToken.Encode(), nil)
	badAwardPageTokenRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badAwardPageTokenRec, badAwardPageTokenReq)
	if badAwardPageTokenRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("awards list bad page_token status: got=%d body=%s", badAwardPageTokenRec.Result().StatusCode, badAwardPageTokenRec.Body.String())
	}
	var badAwardPageTokenResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(badAwardPageTokenRec.Body.Bytes(), &badAwardPageTokenResp); err != nil {
		t.Fatalf("unmarshal awards list bad page_token response: %v", err)
	}
	if badAwardPageTokenResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid awards page_token result code, got=%s", badAwardPageTokenResp.GetMeta().GetResultCode().String())
	}

	qBadUIPageSize := make(url.Values)
	qBadUIPageSize.Set("meta.actor.actorId", "op-1")
	qBadUIPageSize.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadUIPageSize.Set("page_size", "-1")
	badUIPageSizeReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qBadUIPageSize.Encode(), nil)
	badUIPageSizeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badUIPageSizeRec, badUIPageSizeReq)
	if badUIPageSizeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list bad page_size status: got=%d body=%s", badUIPageSizeRec.Result().StatusCode, badUIPageSizeRec.Body.String())
	}
	var badUIPageSizeResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(badUIPageSizeRec.Body.Bytes(), &badUIPageSizeResp); err != nil {
		t.Fatalf("unmarshal ui list bad page_size response: %v", err)
	}
	if badUIPageSizeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid ui page_size result code, got=%s", badUIPageSizeResp.GetMeta().GetResultCode().String())
	}
}
