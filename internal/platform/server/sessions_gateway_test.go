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
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestSessionsGatewayParity_Workflow(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 0, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterSessionsServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register sessions gateway handlers: %v", err)
	}

	startReq := &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	}
	startBody, _ := protojson.Marshal(startReq)
	startHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/sessions:start", bytes.NewReader(startBody))
	startHTTPReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	gwMux.ServeHTTP(startRec, startHTTPReq)
	if startRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("start status: got=%d body=%s", startRec.Result().StatusCode, startRec.Body.String())
	}
	var startResp rgsv1.StartSessionResponse
	if err := protojson.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal start response: %v", err)
	}
	if startResp.Session.GetSessionId() == "" {
		t.Fatalf("expected session id")
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "player-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	getReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+startResp.Session.GetSessionId()+"?"+q.Encode(), nil)
	getRec := httptest.NewRecorder()
	gwMux.ServeHTTP(getRec, getReq)
	if getRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("get status: got=%d body=%s", getRec.Result().StatusCode, getRec.Body.String())
	}
	var getResp rgsv1.GetSessionResponse
	if err := protojson.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getResp.Session.GetSessionId() != startResp.Session.GetSessionId() {
		t.Fatalf("unexpected session id: got=%s want=%s", getResp.Session.GetSessionId(), startResp.Session.GetSessionId())
	}
}

func TestSessionsGatewayActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 10, 0, 0, time.UTC)}
	svc := NewSessionsService(clk)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterSessionsServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register sessions gateway handlers: %v", err)
	}

	startReq := &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	}
	startBody, _ := protojson.Marshal(startReq)
	startHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/sessions:start", bytes.NewReader(startBody))
	startHTTPReq = startHTTPReq.WithContext(platformauth.WithActor(startHTTPReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	startHTTPReq.Header.Set("Content-Type", "application/json")
	startRec := httptest.NewRecorder()
	gwMux.ServeHTTP(startRec, startHTTPReq)
	if startRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("start mismatch status: got=%d body=%s", startRec.Result().StatusCode, startRec.Body.String())
	}
	var startResp rgsv1.StartSessionResponse
	if err := protojson.Unmarshal(startRec.Body.Bytes(), &startResp); err != nil {
		t.Fatalf("unmarshal start mismatch response: %v", err)
	}
	if startResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied start mismatch, got=%v", startResp.GetMeta().GetResultCode())
	}
	if startResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason for start, got=%q", startResp.GetMeta().GetDenialReason())
	}

	seed, err := svc.StartSession(context.Background(), &rgsv1.StartSessionRequest{
		Meta:     meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-1",
		DeviceId: "device-a",
	})
	if err != nil {
		t.Fatalf("seed start session err: %v", err)
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "player-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	getReq := httptest.NewRequest(http.MethodGet, "/v1/sessions/"+seed.Session.GetSessionId()+"?"+q.Encode(), nil)
	getReq = getReq.WithContext(platformauth.WithActor(getReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	getRec := httptest.NewRecorder()
	gwMux.ServeHTTP(getRec, getReq)
	if getRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("get mismatch status: got=%d body=%s", getRec.Result().StatusCode, getRec.Body.String())
	}
	var getResp rgsv1.GetSessionResponse
	if err := protojson.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get mismatch response: %v", err)
	}
	if getResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied get mismatch, got=%v", getResp.GetMeta().GetResultCode())
	}
	if getResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason for get, got=%q", getResp.GetMeta().GetDenialReason())
	}
}
