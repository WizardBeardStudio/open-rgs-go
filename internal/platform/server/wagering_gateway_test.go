package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestWageringGatewayActorMismatchDenied(t *testing.T) {
	svc := NewWageringService(ledgerFixedClock{now: time.Date(2026, 2, 15, 11, 0, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterWageringServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register wagering gateway handlers: %v", err)
	}

	placeReq := &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-http-wager-mismatch-place"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	}
	placeBody, _ := protojson.Marshal(placeReq)
	placeHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/wagering/wagers", bytes.NewReader(placeBody))
	placeHTTPReq = placeHTTPReq.WithContext(platformauth.WithActor(placeHTTPReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	placeHTTPReq.Header.Set("Content-Type", "application/json")
	placeRec := httptest.NewRecorder()
	gwMux.ServeHTTP(placeRec, placeHTTPReq)
	if placeRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("place actor mismatch status: got=%d body=%s", placeRec.Result().StatusCode, placeRec.Body.String())
	}
	var placeResp rgsv1.PlaceWagerResponse
	if err := protojson.Unmarshal(placeRec.Body.Bytes(), &placeResp); err != nil {
		t.Fatalf("unmarshal place actor mismatch response: %v body=%s", err, placeRec.Body.String())
	}
	if placeResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied place result code, got=%v", placeResp.GetMeta().GetResultCode())
	}
	if placeResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for place, got=%q", placeResp.GetMeta().GetDenialReason())
	}

	seed, err := svc.PlaceWager(context.Background(), &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-http-wager-seed"),
		PlayerId: "player-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("seed place wager err: %v", err)
	}

	settleReq := &rgsv1.SettleWagerRequest{
		Meta:       meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, "idem-http-wager-mismatch-settle"),
		WagerId:    seed.Wager.GetWagerId(),
		Payout:     &rgsv1.Money{AmountMinor: 120, Currency: "USD"},
		OutcomeRef: "outcome-http",
	}
	settleBody, _ := protojson.Marshal(settleReq)
	settlePath := "/v1/wagering/wagers/" + seed.Wager.GetWagerId() + ":settle"
	settleHTTPReq := httptest.NewRequest(http.MethodPost, settlePath, bytes.NewReader(settleBody))
	settleHTTPReq = settleHTTPReq.WithContext(platformauth.WithActor(settleHTTPReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	settleHTTPReq.Header.Set("Content-Type", "application/json")
	settleRec := httptest.NewRecorder()
	gwMux.ServeHTTP(settleRec, settleHTTPReq)
	if settleRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("settle actor mismatch status: got=%d body=%s", settleRec.Result().StatusCode, settleRec.Body.String())
	}
	var settleResp rgsv1.SettleWagerResponse
	if err := protojson.Unmarshal(settleRec.Body.Bytes(), &settleResp); err != nil {
		t.Fatalf("unmarshal settle actor mismatch response: %v body=%s", err, settleRec.Body.String())
	}
	if settleResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied settle result code, got=%v", settleResp.GetMeta().GetResultCode())
	}
	if settleResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for settle, got=%q", settleResp.GetMeta().GetDenialReason())
	}

	cancelReq := &rgsv1.CancelWagerRequest{
		Meta:    meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "idem-http-wager-mismatch-cancel"),
		WagerId: seed.Wager.GetWagerId(),
		Reason:  "ops-cancel",
	}
	cancelBody, _ := protojson.Marshal(cancelReq)
	cancelPath := "/v1/wagering/wagers/" + seed.Wager.GetWagerId() + ":cancel"
	cancelHTTPReq := httptest.NewRequest(http.MethodPost, cancelPath, bytes.NewReader(cancelBody))
	cancelHTTPReq = cancelHTTPReq.WithContext(platformauth.WithActor(cancelHTTPReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	cancelHTTPReq.Header.Set("Content-Type", "application/json")
	cancelRec := httptest.NewRecorder()
	gwMux.ServeHTTP(cancelRec, cancelHTTPReq)
	if cancelRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("cancel actor mismatch status: got=%d body=%s", cancelRec.Result().StatusCode, cancelRec.Body.String())
	}
	var cancelResp rgsv1.CancelWagerResponse
	if err := protojson.Unmarshal(cancelRec.Body.Bytes(), &cancelResp); err != nil {
		t.Fatalf("unmarshal cancel actor mismatch response: %v body=%s", err, cancelRec.Body.String())
	}
	if cancelResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied cancel result code, got=%v", cancelResp.GetMeta().GetResultCode())
	}
	if cancelResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for cancel, got=%q", cancelResp.GetMeta().GetDenialReason())
	}
}
