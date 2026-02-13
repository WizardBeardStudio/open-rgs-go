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
