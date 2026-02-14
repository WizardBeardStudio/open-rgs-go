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

func TestIdentityGatewayParity_Workflow(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 14, 0, 0, 0, time.UTC)}, "gateway-secret", 15*time.Minute, time.Hour)
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterIdentityServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register identity gateway handlers: %v", err)
	}

	loginReq := &rgsv1.LoginRequest{
		Meta: meta("player-gw-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-gw-1", Pin: "1234"},
		},
	}
	loginBody, _ := protojson.Marshal(loginReq)
	loginHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/login", bytes.NewReader(loginBody))
	loginHTTPReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	gwMux.ServeHTTP(loginRec, loginHTTPReq)
	if loginRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("login http status: got=%d want=%d body=%s", loginRec.Result().StatusCode, http.StatusOK, loginRec.Body.String())
	}
	var loginResp rgsv1.LoginResponse
	if err := protojson.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}
	if loginResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("unexpected login result: %v", loginResp.Meta.GetResultCode())
	}

	refreshReq := &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-gw-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: loginResp.Token.GetRefreshToken(),
	}
	refreshBody, _ := protojson.Marshal(refreshReq)
	refreshHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/refresh", bytes.NewReader(refreshBody))
	refreshHTTPReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	gwMux.ServeHTTP(refreshRec, refreshHTTPReq)
	if refreshRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("refresh http status: got=%d want=%d body=%s", refreshRec.Result().StatusCode, http.StatusOK, refreshRec.Body.String())
	}
	var refreshResp rgsv1.RefreshTokenResponse
	if err := protojson.Unmarshal(refreshRec.Body.Bytes(), &refreshResp); err != nil {
		t.Fatalf("unmarshal refresh response: %v", err)
	}
	if refreshResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("unexpected refresh result: %v", refreshResp.Meta.GetResultCode())
	}

	logoutReq := &rgsv1.LogoutRequest{
		Meta:         meta("player-gw-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: refreshResp.Token.GetRefreshToken(),
	}
	logoutBody, _ := protojson.Marshal(logoutReq)
	logoutHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/logout", bytes.NewReader(logoutBody))
	logoutHTTPReq.Header.Set("Content-Type", "application/json")
	logoutRec := httptest.NewRecorder()
	gwMux.ServeHTTP(logoutRec, logoutHTTPReq)
	if logoutRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("logout http status: got=%d want=%d body=%s", logoutRec.Result().StatusCode, http.StatusOK, logoutRec.Body.String())
	}
	var logoutResp rgsv1.LogoutResponse
	if err := protojson.Unmarshal(logoutRec.Body.Bytes(), &logoutResp); err != nil {
		t.Fatalf("unmarshal logout response: %v", err)
	}
	if logoutResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("unexpected logout result: %v", logoutResp.Meta.GetResultCode())
	}
}

func TestIdentityGatewaySetCredentialActorMismatchDenied(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 14, 10, 0, 0, time.UTC)}, "gateway-secret", 15*time.Minute, time.Hour)
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterIdentityServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register identity gateway handlers: %v", err)
	}

	reqBody := &rgsv1.SetCredentialRequest{
		Meta:           meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-gw-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "1234"),
		Reason:         "bootstrap",
	}
	body, _ := protojson.Marshal(reqBody)
	req := httptest.NewRequest(http.MethodPost, "/v1/identity/credentials:set", bytes.NewReader(body))
	req = req.WithContext(platformauth.WithActor(req.Context(), platformauth.Actor{
		ID:   "ctx-op",
		Type: "ACTOR_TYPE_OPERATOR",
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("set credential mismatch status: got=%d body=%s", rec.Result().StatusCode, rec.Body.String())
	}
	var resp rgsv1.SetCredentialResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal set credential mismatch response: %v", err)
	}
	if resp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied mismatch, got=%v", resp.GetMeta().GetResultCode())
	}
	if resp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch with token reason, got=%q", resp.GetMeta().GetDenialReason())
	}
}

func TestIdentityGatewayRefreshLogoutActorMismatchDenied(t *testing.T) {
	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 14, 15, 0, 0, time.UTC)}, "gateway-secret", 15*time.Minute, time.Hour)
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterIdentityServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register identity gateway handlers: %v", err)
	}

	loginReq := &rgsv1.LoginRequest{
		Meta: meta("player-gw-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-gw-1", Pin: "1234"},
		},
	}
	loginBody, _ := protojson.Marshal(loginReq)
	loginHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/login", bytes.NewReader(loginBody))
	loginHTTPReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	gwMux.ServeHTTP(loginRec, loginHTTPReq)
	if loginRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("login status: got=%d body=%s", loginRec.Result().StatusCode, loginRec.Body.String())
	}
	var loginResp rgsv1.LoginResponse
	if err := protojson.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("unmarshal login response: %v", err)
	}

	refreshReq := &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-gw-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: loginResp.Token.GetRefreshToken(),
	}
	refreshBody, _ := protojson.Marshal(refreshReq)
	refreshHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/refresh", bytes.NewReader(refreshBody))
	refreshHTTPReq.Header.Set("Content-Type", "application/json")
	refreshRec := httptest.NewRecorder()
	gwMux.ServeHTTP(refreshRec, refreshHTTPReq)
	if refreshRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("refresh mismatch status: got=%d body=%s", refreshRec.Result().StatusCode, refreshRec.Body.String())
	}
	var refreshResp rgsv1.RefreshTokenResponse
	if err := protojson.Unmarshal(refreshRec.Body.Bytes(), &refreshResp); err != nil {
		t.Fatalf("unmarshal refresh mismatch response: %v", err)
	}
	if refreshResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied refresh mismatch, got=%v", refreshResp.GetMeta().GetResultCode())
	}
	if refreshResp.GetMeta().GetDenialReason() != "actor mismatch" {
		t.Fatalf("expected actor mismatch reason on refresh, got=%q", refreshResp.GetMeta().GetDenialReason())
	}

	logoutReq := &rgsv1.LogoutRequest{
		Meta:         meta("player-gw-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: loginResp.Token.GetRefreshToken(),
	}
	logoutBody, _ := protojson.Marshal(logoutReq)
	logoutHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/identity/logout", bytes.NewReader(logoutBody))
	logoutHTTPReq.Header.Set("Content-Type", "application/json")
	logoutRec := httptest.NewRecorder()
	gwMux.ServeHTTP(logoutRec, logoutHTTPReq)
	if logoutRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("logout mismatch status: got=%d body=%s", logoutRec.Result().StatusCode, logoutRec.Body.String())
	}
	var logoutResp rgsv1.LogoutResponse
	if err := protojson.Unmarshal(logoutRec.Body.Bytes(), &logoutResp); err != nil {
		t.Fatalf("unmarshal logout mismatch response: %v", err)
	}
	if logoutResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied logout mismatch, got=%v", logoutResp.GetMeta().GetResultCode())
	}
	if logoutResp.GetMeta().GetDenialReason() != "actor mismatch" {
		t.Fatalf("expected actor mismatch reason on logout, got=%q", logoutResp.GetMeta().GetDenialReason())
	}
}
