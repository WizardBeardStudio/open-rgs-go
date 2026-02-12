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
