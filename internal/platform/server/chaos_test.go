package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func TestChaosLostCommsBufferExhaustionDisablesIngress(t *testing.T) {
	svc := NewEventsService(ledgerFixedClock{now: time.Date(2026, 2, 12, 19, 0, 0, 0, time.UTC)})
	svc.bufferCap = 0

	resp, err := svc.SubmitSignificantEvent(context.Background(), &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:     "ev-chaos-1",
			EquipmentId: "eq-1",
		},
	})
	if err != nil {
		t.Fatalf("submit significant event err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied on buffer exhaustion, got=%v", resp.Meta.GetResultCode())
	}
	if !svc.disabled {
		t.Fatalf("expected service to disable ingress when buffer exhausted")
	}
}

func TestChaosDBFailoverSimulationLedgerFailClosedOnAuditLoss(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 12, 19, 10, 0, 0, time.UTC)})
	svc.AuditStore = nil

	resp, err := svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-chaos", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-chaos"),
		AccountId: "acct-chaos",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("deposit err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_ERROR {
		t.Fatalf("expected error when audit store unavailable, got=%v", resp.Meta.GetResultCode())
	}

	bal, err := svc.GetBalance(context.Background(), &rgsv1.GetBalanceRequest{Meta: meta("acct-chaos", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""), AccountId: "acct-chaos"})
	if err != nil {
		t.Fatalf("get balance err: %v", err)
	}
	if bal.AvailableBalance.GetAmountMinor() != 0 {
		t.Fatalf("expected fail-closed rollback on audit loss, balance=%d", bal.AvailableBalance.GetAmountMinor())
	}
}

func TestChaosPartialPartitionAdminBoundaryDenied(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 19, 20, 0, 0, time.UTC)}, nil, []string{"10.0.0.0/8"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/config/changes:propose", nil)
	req.RemoteAddr = "198.51.100.22:40400"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden during simulated partitioned/untrusted admin access, got=%d", rec.Result().StatusCode)
	}
}
