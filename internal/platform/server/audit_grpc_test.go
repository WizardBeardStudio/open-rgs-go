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
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	platformauth "github.com/wizardbeard/open-rgs-go/internal/platform/auth"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestAuditServiceListsAuditEventsAndRemoteActivities(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	guardStore := ledgerSvc.AuditStore
	guard, err := NewRemoteAccessGuard(clk, guardStore, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}

	wrapped := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/v1/config/history", nil)
	req.RemoteAddr = "127.0.0.1:44444"
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	_, _ = ledgerSvc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-audit", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-1"),
		AccountId: "acct-audit",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})

	auditSvc := NewAuditService(clk, guard, ledgerSvc.AuditStore)
	eventsResp, err := auditSvc.ListAuditEvents(context.Background(), &rgsv1.ListAuditEventsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if len(eventsResp.Events) == 0 {
		t.Fatalf("expected audit events")
	}

	remoteResp, err := auditSvc.ListRemoteAccessActivities(context.Background(), &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list remote activities err: %v", err)
	}
	if len(remoteResp.Activities) == 0 {
		t.Fatalf("expected remote access activity entries")
	}
}

func TestAuditGatewayParity(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 30, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	guard, _ := NewRemoteAccessGuard(clk, ledgerSvc.AuditStore, []string{"127.0.0.1/32"})
	auditSvc := NewAuditService(clk, guard, ledgerSvc.AuditStore)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterAuditServiceHandlerServer(context.Background(), gwMux, auditSvc); err != nil {
		t.Fatalf("register audit gateway handlers: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/events?meta.actor.actorId=op-1&meta.actor.actorType=ACTOR_TYPE_OPERATOR", nil)
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("audit events status: got=%d body=%s", rec.Result().StatusCode, rec.Body.String())
	}
	var out rgsv1.ListAuditEventsResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal list audit events response: %v", err)
	}

	verifyReq := &rgsv1.VerifyAuditChainRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	}
	verifyBody, _ := protojson.Marshal(verifyReq)
	verifyHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/audit/chain:verify", bytes.NewReader(verifyBody))
	verifyHTTPReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	gwMux.ServeHTTP(verifyRec, verifyHTTPReq)
	if verifyRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("audit verify status: got=%d body=%s", verifyRec.Result().StatusCode, verifyRec.Body.String())
	}
	var verifyResp rgsv1.VerifyAuditChainResponse
	if err := protojson.Unmarshal(verifyRec.Body.Bytes(), &verifyResp); err != nil {
		t.Fatalf("unmarshal verify response: %v", err)
	}
	if verifyResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_ERROR {
		t.Fatalf("expected verify error without db, got=%v", verifyResp.Meta.GetResultCode())
	}
}

func TestAuditServiceVerifyAuditChainRequiresDB(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 0, 0, 0, time.UTC)}
	auditSvc := NewAuditService(clk, nil)
	resp, err := auditSvc.VerifyAuditChain(context.Background(), &rgsv1.VerifyAuditChainRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("verify audit chain err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_ERROR || resp.Valid {
		t.Fatalf("expected persistence error and valid=false, got code=%v valid=%v", resp.Meta.GetResultCode(), resp.Valid)
	}
}

func TestAuditServiceVerifyAuditChainRejectsInvalidPartitionDay(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 5, 0, 0, time.UTC)}
	auditSvc := NewAuditService(clk, nil)
	resp, err := auditSvc.VerifyAuditChain(context.Background(), &rgsv1.VerifyAuditChainRequest{
		Meta:         meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PartitionDay: "20260213",
	})
	if err != nil {
		t.Fatalf("verify audit chain err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid partition day, got=%v", resp.Meta.GetResultCode())
	}
	if resp.Valid {
		t.Fatalf("expected valid=false for invalid partition day")
	}
}

func TestAuditServiceListAuditEventsRejectsInvalidPageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 10, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	auditSvc := NewAuditService(clk, nil, ledgerSvc.AuditStore)

	resp, err := auditSvc.ListAuditEvents(context.Background(), &rgsv1.ListAuditEventsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "bad-token",
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid page token, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceListRemoteAccessRejectsInvalidPageToken(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 15, 0, 0, time.UTC)}
	guard, err := NewRemoteAccessGuard(clk, audit.NewInMemoryStore(), []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	auditSvc := NewAuditService(clk, guard)

	resp, err := auditSvc.ListRemoteAccessActivities(context.Background(), &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "-1",
	})
	if err != nil {
		t.Fatalf("list remote access err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid page token, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceListAuditEventsRejectsOversizedPageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 20, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	auditSvc := NewAuditService(clk, nil, ledgerSvc.AuditStore)

	resp, err := auditSvc.ListAuditEvents(context.Background(), &rgsv1.ListAuditEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: maxAuditPageSize + 1,
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid oversized page_size, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceListRemoteAccessRejectsOversizedPageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 25, 0, 0, time.UTC)}
	guard, err := NewRemoteAccessGuard(clk, audit.NewInMemoryStore(), []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	auditSvc := NewAuditService(clk, guard)

	resp, err := auditSvc.ListRemoteAccessActivities(context.Background(), &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: maxAuditPageSize + 1,
	})
	if err != nil {
		t.Fatalf("list remote access err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid oversized page_size, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceListAuditEventsRejectsNegativePageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 30, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	auditSvc := NewAuditService(clk, nil, ledgerSvc.AuditStore)

	resp, err := auditSvc.ListAuditEvents(context.Background(), &rgsv1.ListAuditEventsRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: -1,
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid negative page_size, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceListRemoteAccessRejectsNegativePageSize(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 35, 0, 0, time.UTC)}
	guard, err := NewRemoteAccessGuard(clk, audit.NewInMemoryStore(), []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	auditSvc := NewAuditService(clk, guard)

	resp, err := auditSvc.ListRemoteAccessActivities(context.Background(), &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta:     meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageSize: -1,
	})
	if err != nil {
		t.Fatalf("list remote access err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid negative page_size, got=%v", resp.Meta.GetResultCode())
	}
}

func TestAuditServiceActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 40, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	guard, err := NewRemoteAccessGuard(clk, ledgerSvc.AuditStore, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	auditSvc := NewAuditService(clk, guard, ledgerSvc.AuditStore)
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"})

	listResp, err := auditSvc.ListAuditEvents(ctx, &rgsv1.ListAuditEventsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if listResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list audit events, got=%v", listResp.Meta.GetResultCode())
	}
	if listResp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for list audit events, got=%q", listResp.Meta.GetDenialReason())
	}

	remoteResp, err := auditSvc.ListRemoteAccessActivities(ctx, &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list remote activities err: %v", err)
	}
	if remoteResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list remote activities, got=%v", remoteResp.Meta.GetResultCode())
	}
	if remoteResp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for remote activities, got=%q", remoteResp.Meta.GetDenialReason())
	}

	verifyResp, err := auditSvc.VerifyAuditChain(ctx, &rgsv1.VerifyAuditChainRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("verify chain err: %v", err)
	}
	if verifyResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied verify chain, got=%v", verifyResp.Meta.GetResultCode())
	}
	if verifyResp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial for verify chain, got=%q", verifyResp.Meta.GetDenialReason())
	}
}

func TestAuditGatewayActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 45, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	guard, _ := NewRemoteAccessGuard(clk, ledgerSvc.AuditStore, []string{"127.0.0.1/32"})
	auditSvc := NewAuditService(clk, guard, ledgerSvc.AuditStore)

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterAuditServiceHandlerServer(context.Background(), gwMux, auditSvc); err != nil {
		t.Fatalf("register audit gateway handlers: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/events?meta.actor.actorId=op-1&meta.actor.actorType=ACTOR_TYPE_OPERATOR", nil)
	req = req.WithContext(platformauth.WithActor(req.Context(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"}))
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("audit events mismatch status: got=%d body=%s", rec.Result().StatusCode, rec.Body.String())
	}
	var out rgsv1.ListAuditEventsResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal list audit mismatch response: %v", err)
	}
	if out.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied gateway list audit events, got=%v", out.GetMeta().GetResultCode())
	}
	if out.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason for gateway list audit events, got=%q", out.GetMeta().GetDenialReason())
	}

	remoteReq := httptest.NewRequest(http.MethodGet, "/v1/audit/remote-access?meta.actor.actorId=op-1&meta.actor.actorType=ACTOR_TYPE_OPERATOR", nil)
	remoteReq = remoteReq.WithContext(platformauth.WithActor(remoteReq.Context(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"}))
	remoteRec := httptest.NewRecorder()
	gwMux.ServeHTTP(remoteRec, remoteReq)
	if remoteRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("remote activities mismatch status: got=%d body=%s", remoteRec.Result().StatusCode, remoteRec.Body.String())
	}
	var remoteOut rgsv1.ListRemoteAccessActivitiesResponse
	if err := protojson.Unmarshal(remoteRec.Body.Bytes(), &remoteOut); err != nil {
		t.Fatalf("unmarshal list remote activities mismatch response: %v", err)
	}
	if remoteOut.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied gateway list remote activities, got=%v", remoteOut.GetMeta().GetResultCode())
	}
	if remoteOut.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason for gateway list remote activities, got=%q", remoteOut.GetMeta().GetDenialReason())
	}

	verifyReq := &rgsv1.VerifyAuditChainRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	}
	verifyBody, _ := protojson.Marshal(verifyReq)
	verifyHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/audit/chain:verify", bytes.NewReader(verifyBody))
	verifyHTTPReq = verifyHTTPReq.WithContext(platformauth.WithActor(verifyHTTPReq.Context(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"}))
	verifyHTTPReq.Header.Set("Content-Type", "application/json")
	verifyRec := httptest.NewRecorder()
	gwMux.ServeHTTP(verifyRec, verifyHTTPReq)
	if verifyRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("verify chain mismatch status: got=%d body=%s", verifyRec.Result().StatusCode, verifyRec.Body.String())
	}
	var verifyOut rgsv1.VerifyAuditChainResponse
	if err := protojson.Unmarshal(verifyRec.Body.Bytes(), &verifyOut); err != nil {
		t.Fatalf("unmarshal verify chain mismatch response: %v", err)
	}
	if verifyOut.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied gateway verify chain, got=%v", verifyOut.GetMeta().GetResultCode())
	}
	if verifyOut.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason for gateway verify chain, got=%q", verifyOut.GetMeta().GetDenialReason())
	}
}
