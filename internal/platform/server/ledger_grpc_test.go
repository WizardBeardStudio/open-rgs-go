package server

import (
	"context"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

type ledgerFixedClock struct {
	now time.Time
}

func (f ledgerFixedClock) Now() time.Time {
	return f.now
}

func meta(actorID string, actorType rgsv1.ActorType, idem string) *rgsv1.RequestMeta {
	return &rgsv1.RequestMeta{
		RequestId:      "req-1",
		IdempotencyKey: idem,
		Actor: &rgsv1.Actor{
			ActorId:   actorID,
			ActorType: actorType,
		},
	}
}

func TestLedgerDepositIdempotency(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 15, 0, 0, 0, time.UTC)})

	first, err := svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-1"),
		AccountId: "acct-1",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("first deposit err: %v", err)
	}
	second, err := svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-1"),
		AccountId: "acct-1",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("second deposit err: %v", err)
	}

	if first.Transaction.GetTransactionId() != second.Transaction.GetTransactionId() {
		t.Fatalf("expected same transaction id for idempotent deposit; first=%s second=%s", first.Transaction.GetTransactionId(), second.Transaction.GetTransactionId())
	}

	bal, err := svc.GetBalance(context.Background(), &rgsv1.GetBalanceRequest{Meta: meta("acct-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""), AccountId: "acct-1"})
	if err != nil {
		t.Fatalf("get balance err: %v", err)
	}
	if bal.AvailableBalance.GetAmountMinor() != 1000 {
		t.Fatalf("balance should be credited once; got=%d", bal.AvailableBalance.GetAmountMinor())
	}

	if len(svc.AuditEvents()) == 0 {
		t.Fatalf("expected audit event for successful state change")
	}
}

func TestLedgerWithdrawDeniedOnInsufficientFunds(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 15, 0, 0, 0, time.UTC)})

	_, _ = svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "seed"),
		AccountId: "acct-2",
		Amount:    &rgsv1.Money{AmountMinor: 500, Currency: "USD"},
	})

	resp, err := svc.Withdraw(context.Background(), &rgsv1.WithdrawRequest{
		Meta:      meta("acct-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "w1"),
		AccountId: "acct-2",
		Amount:    &rgsv1.Money{AmountMinor: 700, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("withdraw err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result code, got=%v", resp.Meta.GetResultCode())
	}

	bal, _ := svc.GetBalance(context.Background(), &rgsv1.GetBalanceRequest{Meta: meta("acct-2", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""), AccountId: "acct-2"})
	if bal.AvailableBalance.GetAmountMinor() != 500 {
		t.Fatalf("balance mutated on denied withdraw; got=%d", bal.AvailableBalance.GetAmountMinor())
	}
}

func TestLedgerTransferToDevicePartialAndBalanced(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 15, 0, 0, 0, time.UTC)})

	_, _ = svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("acct-3", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "seed"),
		AccountId: "acct-3",
		Amount:    &rgsv1.Money{AmountMinor: 800, Currency: "USD"},
	})

	resp, err := svc.TransferToDevice(context.Background(), &rgsv1.TransferToDeviceRequest{
		Meta:            meta("acct-3", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "td-1"),
		AccountId:       "acct-3",
		DeviceId:        "device-1",
		RequestedAmount: &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("transfer to device err: %v", err)
	}
	if resp.TransferStatus != rgsv1.TransferStatus_TRANSFER_STATUS_PARTIAL {
		t.Fatalf("expected partial status, got=%v", resp.TransferStatus)
	}
	if resp.TransferredAmount.GetAmountMinor() != 800 {
		t.Fatalf("expected partial amount 800, got=%d", resp.TransferredAmount.GetAmountMinor())
	}

	bal, _ := svc.GetBalance(context.Background(), &rgsv1.GetBalanceRequest{Meta: meta("acct-3", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""), AccountId: "acct-3"})
	if bal.AvailableBalance.GetAmountMinor() != 0 {
		t.Fatalf("expected account drained to 0, got=%d", bal.AvailableBalance.GetAmountMinor())
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	for txID, postings := range svc.postingsByTx {
		if !isBalanced(postings) {
			t.Fatalf("transaction %s has unbalanced postings", txID)
		}
	}
}

func TestLedgerAuthorizationDeniedForForeignPlayerAccount(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 15, 0, 0, 0, time.UTC)})

	resp, err := svc.Deposit(context.Background(), &rgsv1.DepositRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-foreign"),
		AccountId: "acct-foreign",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("deposit err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied result for unauthorized player action, got=%v", resp.Meta.GetResultCode())
	}

	events := svc.AuditEvents()
	if len(events) == 0 {
		t.Fatalf("expected denied action to be audited")
	}
	if events[len(events)-1].Result != "denied" {
		t.Fatalf("expected denied audit result, got=%s", events[len(events)-1].Result)
	}
}
