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
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"google.golang.org/protobuf/encoding/protojson"
)

func assertGatewayMetaFields(t *testing.T, m *rgsv1.ResponseMeta, wantRequestID string) {
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
	if awardResp.GetMeta().GetDenialReason() != "award requires player_id, award_type, and positive amount" {
		t.Fatalf("expected award denial reason for invalid request, got=%q", awardResp.GetMeta().GetDenialReason())
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
	if bonusResp.GetMeta().GetDenialReason() != "invalid occurred_at" {
		t.Fatalf("expected bonus denial reason invalid occurred_at, got=%q", bonusResp.GetMeta().GetDenialReason())
	}
	assertGatewayMetaFields(t, bonusResp.GetMeta(), "req-1")

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
	if awardBadTimeResp.GetMeta().GetDenialReason() != "invalid occurred_at" {
		t.Fatalf("expected award denial reason invalid occurred_at, got=%q", awardBadTimeResp.GetMeta().GetDenialReason())
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
	if uiResp.GetMeta().GetDenialReason() != "invalid event_time" {
		t.Fatalf("expected ui denial reason invalid event_time, got=%q", uiResp.GetMeta().GetDenialReason())
	}

	uiBadTypeReq := &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType(99),
		},
	}
	uiBadTypeBody, _ := protojson.Marshal(uiBadTypeReq)
	uiBadTypeHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/ui/system-window-events", bytes.NewReader(uiBadTypeBody))
	uiBadTypeHTTPReq.Header.Set("Content-Type", "application/json")
	uiBadTypeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(uiBadTypeRec, uiBadTypeHTTPReq)
	if uiBadTypeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui bad type submit status: got=%d body=%s", uiBadTypeRec.Result().StatusCode, uiBadTypeRec.Body.String())
	}
	var uiBadTypeResp rgsv1.SubmitSystemWindowEventResponse
	if err := protojson.Unmarshal(uiBadTypeRec.Body.Bytes(), &uiBadTypeResp); err != nil {
		t.Fatalf("unmarshal ui bad type response: %v", err)
	}
	if uiBadTypeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid ui bad type result code, got=%s", uiBadTypeResp.GetMeta().GetResultCode().String())
	}
	if uiBadTypeResp.GetMeta().GetDenialReason() != "event requires equipment_id, window_id, and event_type" {
		t.Fatalf("expected ui bad-type denial reason, got=%q", uiBadTypeResp.GetMeta().GetDenialReason())
	}

	playerBonusReq := &rgsv1.RecordBonusTransactionRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Transaction: &rgsv1.BonusTransaction{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			Amount:      &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
		},
	}
	playerBonusBody, _ := protojson.Marshal(playerBonusReq)
	playerBonusHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/bonus-transactions", bytes.NewReader(playerBonusBody))
	playerBonusHTTPReq.Header.Set("Content-Type", "application/json")
	playerBonusRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerBonusRec, playerBonusHTTPReq)
	if playerBonusRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player bonus record status: got=%d body=%s", playerBonusRec.Result().StatusCode, playerBonusRec.Body.String())
	}
	var playerBonusResp rgsv1.RecordBonusTransactionResponse
	if err := protojson.Unmarshal(playerBonusRec.Body.Bytes(), &playerBonusResp); err != nil {
		t.Fatalf("unmarshal player bonus response: %v", err)
	}
	if playerBonusResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player bonus result code, got=%s", playerBonusResp.GetMeta().GetResultCode().String())
	}
	if playerBonusResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player bonus denial reason unauthorized actor type, got=%q", playerBonusResp.GetMeta().GetDenialReason())
	}
	assertGatewayMetaFields(t, playerBonusResp.GetMeta(), "req-1")

	playerUIReq := &rgsv1.SubmitSystemWindowEventRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Event: &rgsv1.SystemWindowEvent{
			EquipmentId: "eq-1",
			PlayerId:    "player-1",
			WindowId:    "sys-menu",
			EventType:   rgsv1.SystemWindowEventType_SYSTEM_WINDOW_EVENT_TYPE_OPENED,
		},
	}
	playerUIBody, _ := protojson.Marshal(playerUIReq)
	playerUIHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/ui/system-window-events", bytes.NewReader(playerUIBody))
	playerUIHTTPReq.Header.Set("Content-Type", "application/json")
	playerUIRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerUIRec, playerUIHTTPReq)
	if playerUIRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player ui submit status: got=%d body=%s", playerUIRec.Result().StatusCode, playerUIRec.Body.String())
	}
	var playerUIResp rgsv1.SubmitSystemWindowEventResponse
	if err := protojson.Unmarshal(playerUIRec.Body.Bytes(), &playerUIResp); err != nil {
		t.Fatalf("unmarshal player ui response: %v", err)
	}
	if playerUIResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player ui result code, got=%s", playerUIResp.GetMeta().GetResultCode().String())
	}
	if playerUIResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player ui denial reason unauthorized actor type, got=%q", playerUIResp.GetMeta().GetDenialReason())
	}

	playerAwardReq := &rgsv1.RecordPromotionalAwardRequest{
		Meta: meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Award: &rgsv1.PromotionalAward{
			PlayerId:   "player-1",
			CampaignId: "camp-1",
			AwardType:  rgsv1.PromotionalAwardType_PROMOTIONAL_AWARD_TYPE_FREEPLAY,
			Amount:     &rgsv1.Money{AmountMinor: 50, Currency: "USD"},
		},
	}
	playerAwardBody, _ := protojson.Marshal(playerAwardReq)
	playerAwardHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/promotions/awards", bytes.NewReader(playerAwardBody))
	playerAwardHTTPReq.Header.Set("Content-Type", "application/json")
	playerAwardRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerAwardRec, playerAwardHTTPReq)
	if playerAwardRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player award record status: got=%d body=%s", playerAwardRec.Result().StatusCode, playerAwardRec.Body.String())
	}
	var playerAwardResp rgsv1.RecordPromotionalAwardResponse
	if err := protojson.Unmarshal(playerAwardRec.Body.Bytes(), &playerAwardResp); err != nil {
		t.Fatalf("unmarshal player award response: %v", err)
	}
	if playerAwardResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player award result code, got=%s", playerAwardResp.GetMeta().GetResultCode().String())
	}
	if playerAwardResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player award denial reason unauthorized actor type, got=%q", playerAwardResp.GetMeta().GetDenialReason())
	}

	qPlayerBonusList := make(url.Values)
	qPlayerBonusList.Set("meta.actor.actorId", "player-1")
	qPlayerBonusList.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	playerBonusListReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/bonus-transactions?"+qPlayerBonusList.Encode(), nil)
	playerBonusListRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerBonusListRec, playerBonusListReq)
	if playerBonusListRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player bonus list status: got=%d body=%s", playerBonusListRec.Result().StatusCode, playerBonusListRec.Body.String())
	}
	var playerBonusListResp rgsv1.ListRecentBonusTransactionsResponse
	if err := protojson.Unmarshal(playerBonusListRec.Body.Bytes(), &playerBonusListResp); err != nil {
		t.Fatalf("unmarshal player bonus list response: %v", err)
	}
	if playerBonusListResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player bonus list result code, got=%s", playerBonusListResp.GetMeta().GetResultCode().String())
	}
	if playerBonusListResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player bonus list denial reason unauthorized actor type, got=%q", playerBonusListResp.GetMeta().GetDenialReason())
	}

	qPlayerAwardsList := make(url.Values)
	qPlayerAwardsList.Set("meta.actor.actorId", "player-1")
	qPlayerAwardsList.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	playerAwardsListReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qPlayerAwardsList.Encode(), nil)
	playerAwardsListRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerAwardsListRec, playerAwardsListReq)
	if playerAwardsListRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player awards list status: got=%d body=%s", playerAwardsListRec.Result().StatusCode, playerAwardsListRec.Body.String())
	}
	var playerAwardsListResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(playerAwardsListRec.Body.Bytes(), &playerAwardsListResp); err != nil {
		t.Fatalf("unmarshal player awards list response: %v", err)
	}
	if playerAwardsListResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player awards list result code, got=%s", playerAwardsListResp.GetMeta().GetResultCode().String())
	}
	if playerAwardsListResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player awards list denial reason unauthorized actor type, got=%q", playerAwardsListResp.GetMeta().GetDenialReason())
	}

	qPlayerUIList := make(url.Values)
	qPlayerUIList.Set("meta.actor.actorId", "player-1")
	qPlayerUIList.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	playerUIListReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qPlayerUIList.Encode(), nil)
	playerUIListRec := httptest.NewRecorder()
	gwMux.ServeHTTP(playerUIListRec, playerUIListReq)
	if playerUIListRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("player ui list status: got=%d body=%s", playerUIListRec.Result().StatusCode, playerUIListRec.Body.String())
	}
	var playerUIListResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(playerUIListRec.Body.Bytes(), &playerUIListResp); err != nil {
		t.Fatalf("unmarshal player ui list response: %v", err)
	}
	if playerUIListResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied player ui list result code, got=%s", playerUIListResp.GetMeta().GetResultCode().String())
	}
	if playerUIListResp.GetMeta().GetDenialReason() != "unauthorized actor type" {
		t.Fatalf("expected player ui list denial reason unauthorized actor type, got=%q", playerUIListResp.GetMeta().GetDenialReason())
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
	if badPageResp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected bad page token denial reason, got=%q", badPageResp.GetMeta().GetDenialReason())
	}
	qNegativePageToken := make(url.Values)
	qNegativePageToken.Set("meta.actor.actorId", "op-1")
	qNegativePageToken.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qNegativePageToken.Set("page_token", "-1")
	negativePageReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qNegativePageToken.Encode(), nil)
	negativePageRec := httptest.NewRecorder()
	gwMux.ServeHTTP(negativePageRec, negativePageReq)
	if negativePageRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list negative page token status: got=%d body=%s", negativePageRec.Result().StatusCode, negativePageRec.Body.String())
	}
	var negativePageResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(negativePageRec.Body.Bytes(), &negativePageResp); err != nil {
		t.Fatalf("unmarshal ui list negative page token response: %v", err)
	}
	if negativePageResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid negative page token result code, got=%s", negativePageResp.GetMeta().GetResultCode().String())
	}
	if negativePageResp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected negative page token denial reason, got=%q", negativePageResp.GetMeta().GetDenialReason())
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
	if badRangeResp.GetMeta().GetDenialReason() != "from_time must be <= to_time" {
		t.Fatalf("expected bad range denial reason, got=%q", badRangeResp.GetMeta().GetDenialReason())
	}

	qBadFrom := make(url.Values)
	qBadFrom.Set("meta.actor.actorId", "op-1")
	qBadFrom.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadFrom.Set("from_time", "not-a-time")
	badFromReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qBadFrom.Encode(), nil)
	badFromRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badFromRec, badFromReq)
	if badFromRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list bad from_time status: got=%d body=%s", badFromRec.Result().StatusCode, badFromRec.Body.String())
	}
	var badFromResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(badFromRec.Body.Bytes(), &badFromResp); err != nil {
		t.Fatalf("unmarshal ui list bad from_time response: %v", err)
	}
	if badFromResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid bad from_time result code, got=%s", badFromResp.GetMeta().GetResultCode().String())
	}
	if badFromResp.GetMeta().GetDenialReason() != "invalid from_time" {
		t.Fatalf("expected bad from_time denial reason, got=%q", badFromResp.GetMeta().GetDenialReason())
	}

	qBadTo := make(url.Values)
	qBadTo.Set("meta.actor.actorId", "op-1")
	qBadTo.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qBadTo.Set("to_time", "not-a-time")
	badToReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qBadTo.Encode(), nil)
	badToRec := httptest.NewRecorder()
	gwMux.ServeHTTP(badToRec, badToReq)
	if badToRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list bad to_time status: got=%d body=%s", badToRec.Result().StatusCode, badToRec.Body.String())
	}
	var badToResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(badToRec.Body.Bytes(), &badToResp); err != nil {
		t.Fatalf("unmarshal ui list bad to_time response: %v", err)
	}
	if badToResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid bad to_time result code, got=%s", badToResp.GetMeta().GetResultCode().String())
	}
	if badToResp.GetMeta().GetDenialReason() != "invalid to_time" {
		t.Fatalf("expected bad to_time denial reason, got=%q", badToResp.GetMeta().GetDenialReason())
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
	if badBonusLimitResp.GetMeta().GetDenialReason() != "invalid limit" {
		t.Fatalf("expected bonus limit denial reason, got=%q", badBonusLimitResp.GetMeta().GetDenialReason())
	}

	qOversizedBonusLimit := make(url.Values)
	qOversizedBonusLimit.Set("meta.actor.actorId", "op-1")
	qOversizedBonusLimit.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qOversizedBonusLimit.Set("limit", "101")
	oversizedBonusLimitReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/bonus-transactions?"+qOversizedBonusLimit.Encode(), nil)
	oversizedBonusLimitRec := httptest.NewRecorder()
	gwMux.ServeHTTP(oversizedBonusLimitRec, oversizedBonusLimitReq)
	if oversizedBonusLimitRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("bonus list oversized limit status: got=%d body=%s", oversizedBonusLimitRec.Result().StatusCode, oversizedBonusLimitRec.Body.String())
	}
	var oversizedBonusLimitResp rgsv1.ListRecentBonusTransactionsResponse
	if err := protojson.Unmarshal(oversizedBonusLimitRec.Body.Bytes(), &oversizedBonusLimitResp); err != nil {
		t.Fatalf("unmarshal bonus list oversized limit response: %v", err)
	}
	if oversizedBonusLimitResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid oversized bonus limit result code, got=%s", oversizedBonusLimitResp.GetMeta().GetResultCode().String())
	}
	if oversizedBonusLimitResp.GetMeta().GetDenialReason() != "invalid limit" {
		t.Fatalf("expected oversized bonus limit denial reason, got=%q", oversizedBonusLimitResp.GetMeta().GetDenialReason())
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
	if badAwardPageSizeResp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected awards page_size denial reason, got=%q", badAwardPageSizeResp.GetMeta().GetDenialReason())
	}

	qOversizedAwardPageSize := make(url.Values)
	qOversizedAwardPageSize.Set("meta.actor.actorId", "op-1")
	qOversizedAwardPageSize.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qOversizedAwardPageSize.Set("page_size", "101")
	oversizedAwardPageSizeReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qOversizedAwardPageSize.Encode(), nil)
	oversizedAwardPageSizeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(oversizedAwardPageSizeRec, oversizedAwardPageSizeReq)
	if oversizedAwardPageSizeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("awards list oversized page_size status: got=%d body=%s", oversizedAwardPageSizeRec.Result().StatusCode, oversizedAwardPageSizeRec.Body.String())
	}
	var oversizedAwardPageSizeResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(oversizedAwardPageSizeRec.Body.Bytes(), &oversizedAwardPageSizeResp); err != nil {
		t.Fatalf("unmarshal awards list oversized page_size response: %v", err)
	}
	if oversizedAwardPageSizeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid oversized awards page_size result code, got=%s", oversizedAwardPageSizeResp.GetMeta().GetResultCode().String())
	}
	if oversizedAwardPageSizeResp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected oversized awards page_size denial reason, got=%q", oversizedAwardPageSizeResp.GetMeta().GetDenialReason())
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
	if badAwardPageTokenResp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected awards page_token denial reason, got=%q", badAwardPageTokenResp.GetMeta().GetDenialReason())
	}
	qNegativeAwardPageToken := make(url.Values)
	qNegativeAwardPageToken.Set("meta.actor.actorId", "op-1")
	qNegativeAwardPageToken.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qNegativeAwardPageToken.Set("page_token", "-1")
	negativeAwardPageTokenReq := httptest.NewRequest(http.MethodGet, "/v1/promotions/awards?"+qNegativeAwardPageToken.Encode(), nil)
	negativeAwardPageTokenRec := httptest.NewRecorder()
	gwMux.ServeHTTP(negativeAwardPageTokenRec, negativeAwardPageTokenReq)
	if negativeAwardPageTokenRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("awards list negative page_token status: got=%d body=%s", negativeAwardPageTokenRec.Result().StatusCode, negativeAwardPageTokenRec.Body.String())
	}
	var negativeAwardPageTokenResp rgsv1.ListPromotionalAwardsResponse
	if err := protojson.Unmarshal(negativeAwardPageTokenRec.Body.Bytes(), &negativeAwardPageTokenResp); err != nil {
		t.Fatalf("unmarshal awards list negative page_token response: %v", err)
	}
	if negativeAwardPageTokenResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid negative awards page_token result code, got=%s", negativeAwardPageTokenResp.GetMeta().GetResultCode().String())
	}
	if negativeAwardPageTokenResp.GetMeta().GetDenialReason() != "invalid page_token" {
		t.Fatalf("expected negative awards page_token denial reason, got=%q", negativeAwardPageTokenResp.GetMeta().GetDenialReason())
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
	if badUIPageSizeResp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected ui page_size denial reason, got=%q", badUIPageSizeResp.GetMeta().GetDenialReason())
	}

	qOversizedUIPageSize := make(url.Values)
	qOversizedUIPageSize.Set("meta.actor.actorId", "op-1")
	qOversizedUIPageSize.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	qOversizedUIPageSize.Set("page_size", "201")
	oversizedUIPageSizeReq := httptest.NewRequest(http.MethodGet, "/v1/ui/system-window-events?"+qOversizedUIPageSize.Encode(), nil)
	oversizedUIPageSizeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(oversizedUIPageSizeRec, oversizedUIPageSizeReq)
	if oversizedUIPageSizeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("ui list oversized page_size status: got=%d body=%s", oversizedUIPageSizeRec.Result().StatusCode, oversizedUIPageSizeRec.Body.String())
	}
	var oversizedUIPageSizeResp rgsv1.ListSystemWindowEventsResponse
	if err := protojson.Unmarshal(oversizedUIPageSizeRec.Body.Bytes(), &oversizedUIPageSizeResp); err != nil {
		t.Fatalf("unmarshal ui list oversized page_size response: %v", err)
	}
	if oversizedUIPageSizeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid oversized ui page_size result code, got=%s", oversizedUIPageSizeResp.GetMeta().GetResultCode().String())
	}
	if oversizedUIPageSizeResp.GetMeta().GetDenialReason() != "invalid page_size" {
		t.Fatalf("expected oversized ui page_size denial reason, got=%q", oversizedUIPageSizeResp.GetMeta().GetDenialReason())
	}

	promoEvents := promoSvc.AuditStore.Events()
	if !hasAuditEvent(promoEvents, "record_bonus_transaction", audit.ResultDenied) {
		t.Fatalf("expected denied promo audit for invalid/unauthorized bonus path, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "record_bonus_transaction", audit.ResultDenied, "invalid occurred_at") {
		t.Fatalf("expected promo audit reason invalid occurred_at, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "record_bonus_transaction", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected promo audit reason unauthorized actor type for bonus write, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "record_promotional_award", audit.ResultDenied, "invalid request") {
		t.Fatalf("expected promo audit reason invalid request for award write, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "record_promotional_award", audit.ResultDenied, "invalid occurred_at") {
		t.Fatalf("expected promo audit reason invalid occurred_at for award write, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "record_promotional_award", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected promo audit reason unauthorized actor type for award write, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "list_recent_bonus_transactions", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected promo audit reason unauthorized actor type for bonus list, got=%v", promoEvents)
	}
	if !hasAuditEvent(promoEvents, "list_promotional_awards", audit.ResultDenied) {
		t.Fatalf("expected denied promo audit for invalid/unauthorized awards list path, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "list_promotional_awards", audit.ResultDenied, "invalid page_token") {
		t.Fatalf("expected promo audit reason invalid page_token, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "list_promotional_awards", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected promo audit reason unauthorized actor type for awards list, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "list_recent_bonus_transactions", audit.ResultDenied, "invalid limit") {
		t.Fatalf("expected promo audit reason invalid limit, got=%v", promoEvents)
	}
	if !hasAuditEventWithReason(promoEvents, "list_promotional_awards", audit.ResultDenied, "invalid page_size") {
		t.Fatalf("expected promo audit reason invalid page_size, got=%v", promoEvents)
	}

	uiEvents := uiSvc.AuditStore.Events()
	if !hasAuditEvent(uiEvents, "submit_system_window_event", audit.ResultDenied) {
		t.Fatalf("expected denied ui audit for invalid/unauthorized submit path, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "submit_system_window_event", audit.ResultDenied, "invalid event_time") {
		t.Fatalf("expected ui audit reason invalid event_time, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "submit_system_window_event", audit.ResultDenied, "invalid request") {
		t.Fatalf("expected ui audit reason invalid request for submit, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "submit_system_window_event", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected ui audit reason unauthorized actor type for submit, got=%v", uiEvents)
	}
	if !hasAuditEvent(uiEvents, "list_system_window_events", audit.ResultDenied) {
		t.Fatalf("expected denied ui audit for invalid/unauthorized list path, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "invalid page_token") {
		t.Fatalf("expected ui audit reason invalid page_token, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "unauthorized actor type") {
		t.Fatalf("expected ui audit reason unauthorized actor type for list, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "invalid page_size") {
		t.Fatalf("expected ui audit reason invalid page_size, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "invalid from_time") {
		t.Fatalf("expected ui audit reason invalid from_time, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "invalid to_time") {
		t.Fatalf("expected ui audit reason invalid to_time, got=%v", uiEvents)
	}
	if !hasAuditEventWithReason(uiEvents, "list_system_window_events", audit.ResultDenied, "invalid time range") {
		t.Fatalf("expected ui audit reason invalid time range, got=%v", uiEvents)
	}
}

func hasAuditEvent(events []audit.Event, action string, result audit.Result) bool {
	for _, ev := range events {
		if ev.Action == action && ev.Result == result {
			return true
		}
	}
	return false
}

func hasAuditEventWithReason(events []audit.Event, action string, result audit.Result, reason string) bool {
	for _, ev := range events {
		if ev.Action == action && ev.Result == result && ev.Reason == reason {
			return true
		}
	}
	return false
}
