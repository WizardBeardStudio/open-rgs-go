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

func TestLedgerGatewayParity_DepositAndBalance(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 16, 0, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterLedgerServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register ledger gateway handlers: %v", err)
	}

	depReq := &rgsv1.DepositRequest{
		Meta:      meta("acct-10", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-http-1"),
		AccountId: "acct-10",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	}
	depJSON, err := protojson.Marshal(depReq)
	if err != nil {
		t.Fatalf("marshal deposit req: %v", err)
	}

	httpReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/deposits", bytes.NewReader(depJSON))
	httpReq.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, httpReq)

	httpResp := rec.Result()
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		t.Fatalf("deposit http status: got=%d want=%d", httpResp.StatusCode, http.StatusOK)
	}

	var viaHTTP rgsv1.DepositResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &viaHTTP); err != nil {
		t.Fatalf("unmarshal deposit response: %v body=%s", err, rec.Body.String())
	}
	if viaHTTP.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("deposit over http not ok: %v", viaHTTP.Meta.GetResultCode())
	}

	direct, err := svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-10", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-http-1"),
		AccountId: "acct-10",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("direct deposit err: %v", err)
	}

	if viaHTTP.Transaction.GetTransactionId() != direct.Transaction.GetTransactionId() {
		t.Fatalf("gateway/direct parity mismatch transaction id: http=%s direct=%s", viaHTTP.Transaction.GetTransactionId(), direct.Transaction.GetTransactionId())
	}
	if viaHTTP.AvailableBalance.GetAmountMinor() != direct.AvailableBalance.GetAmountMinor() {
		t.Fatalf("gateway/direct parity mismatch available balance: http=%d direct=%d", viaHTTP.AvailableBalance.GetAmountMinor(), direct.AvailableBalance.GetAmountMinor())
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "acct-10")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	balancePath := "/v1/ledger/accounts/acct-10/balance?" + q.Encode()
	balReq := httptest.NewRequest(http.MethodGet, balancePath, nil)
	balRec := httptest.NewRecorder()
	gwMux.ServeHTTP(balRec, balReq)

	if balRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("balance http status: got=%d want=%d", balRec.Result().StatusCode, http.StatusOK)
	}
	var balResp rgsv1.GetBalanceResponse
	if err := protojson.Unmarshal(balRec.Body.Bytes(), &balResp); err != nil {
		t.Fatalf("unmarshal balance response: %v body=%s", err, balRec.Body.String())
	}
	if balResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("balance over http not ok: %v", balResp.Meta.GetResultCode())
	}
	if balResp.AvailableBalance.GetAmountMinor() != 1200 {
		t.Fatalf("unexpected balance over http: got=%d", balResp.AvailableBalance.GetAmountMinor())
	}
}

func TestLedgerGatewayParity_ListTransactions(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 16, 5, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterLedgerServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register ledger gateway handlers: %v", err)
	}

	_, _ = svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-11", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-list-1"),
		AccountId: "acct-11",
		Amount:    &rgsv1.Money{AmountMinor: 300, Currency: "USD"},
	})

	q := make(url.Values)
	q.Set("meta.actor.actorId", "acct-11")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	q.Set("pageSize", "10")
	path := "/v1/ledger/accounts/acct-11/transactions?" + q.Encode()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("list transactions http status: got=%d want=%d", rec.Result().StatusCode, http.StatusOK)
	}
	var viaHTTP rgsv1.ListTransactionsResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &viaHTTP); err != nil {
		t.Fatalf("unmarshal list transactions response: %v body=%s", err, rec.Body.String())
	}
	if viaHTTP.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("list transactions over http not ok: %v", viaHTTP.Meta.GetResultCode())
	}

	direct, err := svc.ListTransactions(context.Background(), &rgsv1.ListTransactionsRequest{
		Meta:      meta("acct-11", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		AccountId: "acct-11",
		PageSize:  10,
	})
	if err != nil {
		t.Fatalf("direct list transactions err: %v", err)
	}
	if len(viaHTTP.Transactions) != len(direct.Transactions) {
		t.Fatalf("gateway/direct list parity mismatch len: http=%d direct=%d", len(viaHTTP.Transactions), len(direct.Transactions))
	}
	if len(viaHTTP.Transactions) > 0 && viaHTTP.Transactions[0].GetTransactionId() != direct.Transactions[0].GetTransactionId() {
		t.Fatalf("gateway/direct list parity mismatch first tx id: http=%s direct=%s", viaHTTP.Transactions[0].GetTransactionId(), direct.Transactions[0].GetTransactionId())
	}
}

func TestLedgerGatewayActorMismatchDenied(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 16, 10, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterLedgerServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register ledger gateway handlers: %v", err)
	}

	depReq := &rgsv1.DepositRequest{
		Meta:      meta("acct-20", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-http-mismatch"),
		AccountId: "acct-20",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	}
	depJSON, err := protojson.Marshal(depReq)
	if err != nil {
		t.Fatalf("marshal deposit req: %v", err)
	}
	depHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/ledger/deposits", bytes.NewReader(depJSON))
	depHTTPReq = depHTTPReq.WithContext(platformauth.WithActor(depHTTPReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	depHTTPReq.Header.Set("Content-Type", "application/json")
	depRec := httptest.NewRecorder()
	gwMux.ServeHTTP(depRec, depHTTPReq)
	if depRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("deposit actor mismatch status: got=%d body=%s", depRec.Result().StatusCode, depRec.Body.String())
	}
	var depResp rgsv1.DepositResponse
	if err := protojson.Unmarshal(depRec.Body.Bytes(), &depResp); err != nil {
		t.Fatalf("unmarshal actor mismatch deposit response: %v body=%s", err, depRec.Body.String())
	}
	if depResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied actor mismatch deposit, got=%v", depResp.Meta.GetResultCode())
	}
	if depResp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason, got=%q", depResp.Meta.GetDenialReason())
	}

	q := make(url.Values)
	q.Set("meta.request_id", "req-balance-mismatch")
	q.Set("meta.actor.actorId", "acct-20")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_PLAYER")
	balReq := httptest.NewRequest(http.MethodGet, "/v1/ledger/accounts/acct-20/balance?"+q.Encode(), nil)
	balReq = balReq.WithContext(platformauth.WithActor(balReq.Context(), platformauth.Actor{
		ID:   "ctx-player",
		Type: "ACTOR_TYPE_PLAYER",
	}))
	balRec := httptest.NewRecorder()
	gwMux.ServeHTTP(balRec, balReq)
	if balRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("balance actor mismatch status: got=%d body=%s", balRec.Result().StatusCode, balRec.Body.String())
	}
	var balResp rgsv1.GetBalanceResponse
	if err := protojson.Unmarshal(balRec.Body.Bytes(), &balResp); err != nil {
		t.Fatalf("unmarshal actor mismatch balance response: %v body=%s", err, balRec.Body.String())
	}
	if balResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied actor mismatch balance, got=%v", balResp.Meta.GetResultCode())
	}
	if balResp.Meta.GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch denial reason on balance, got=%q", balResp.Meta.GetDenialReason())
	}
}
