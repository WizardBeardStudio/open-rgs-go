package server

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func TestLedgerIdempotencyAcrossFinancialOps(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 18, 0, 0, 0, time.UTC)})
	ctx := context.Background()

	_, _ = svc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "seed-dep"),
		AccountId: "acct-idem",
		Amount:    &rgsv1.Money{AmountMinor: 5000, Currency: "USD"},
	})

	w1, err := svc.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-withdraw"),
		AccountId: "acct-idem",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("withdraw first call err: %v", err)
	}
	w2, err := svc.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-withdraw"),
		AccountId: "acct-idem",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("withdraw second call err: %v", err)
	}
	if w1.Transaction.GetTransactionId() != w2.Transaction.GetTransactionId() {
		t.Fatalf("withdraw idempotency broken: first=%s second=%s", w1.Transaction.GetTransactionId(), w2.Transaction.GetTransactionId())
	}

	td1, err := svc.TransferToDevice(ctx, &rgsv1.TransferToDeviceRequest{
		Meta:            meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-to-device"),
		AccountId:       "acct-idem",
		DeviceId:        "device-idem",
		RequestedAmount: &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("transfer to device first call err: %v", err)
	}
	td2, err := svc.TransferToDevice(ctx, &rgsv1.TransferToDeviceRequest{
		Meta:            meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-to-device"),
		AccountId:       "acct-idem",
		DeviceId:        "device-idem",
		RequestedAmount: &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("transfer to device second call err: %v", err)
	}
	if td1.TransferId != td2.TransferId {
		t.Fatalf("transfer-to-device idempotency broken: first=%s second=%s", td1.TransferId, td2.TransferId)
	}

	ta1, err := svc.TransferToAccount(ctx, &rgsv1.TransferToAccountRequest{
		Meta:      meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-to-account"),
		AccountId: "acct-idem",
		Amount:    &rgsv1.Money{AmountMinor: 700, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("transfer to account first call err: %v", err)
	}
	ta2, err := svc.TransferToAccount(ctx, &rgsv1.TransferToAccountRequest{
		Meta:      meta("acct-idem", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-to-account"),
		AccountId: "acct-idem",
		Amount:    &rgsv1.Money{AmountMinor: 700, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("transfer to account second call err: %v", err)
	}
	if ta1.Transaction.GetTransactionId() != ta2.Transaction.GetTransactionId() {
		t.Fatalf("transfer-to-account idempotency broken: first=%s second=%s", ta1.Transaction.GetTransactionId(), ta2.Transaction.GetTransactionId())
	}
}

func TestLedgerRandomizedInvariants_NoNegativeAndBalanced(t *testing.T) {
	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 11, 18, 5, 0, 0, time.UTC)})
	ctx := context.Background()
	r := rand.New(rand.NewSource(7))
	accountID := "acct-prop"

	for i := 0; i < 300; i++ {
		amount := int64(r.Intn(1500) + 1)
		op := r.Intn(4)
		idem := fmt.Sprintf("prop-%d", i)

		switch op {
		case 0:
			_, _ = svc.Deposit(ctx, &rgsv1.DepositRequest{
				Meta:      meta(accountID, rgsv1.ActorType_ACTOR_TYPE_PLAYER, idem),
				AccountId: accountID,
				Amount:    &rgsv1.Money{AmountMinor: amount, Currency: "USD"},
			})
		case 1:
			_, _ = svc.Withdraw(ctx, &rgsv1.WithdrawRequest{
				Meta:      meta(accountID, rgsv1.ActorType_ACTOR_TYPE_PLAYER, idem),
				AccountId: accountID,
				Amount:    &rgsv1.Money{AmountMinor: amount, Currency: "USD"},
			})
		case 2:
			_, _ = svc.TransferToDevice(ctx, &rgsv1.TransferToDeviceRequest{
				Meta:            meta(accountID, rgsv1.ActorType_ACTOR_TYPE_PLAYER, idem),
				AccountId:       accountID,
				DeviceId:        "device-prop",
				RequestedAmount: &rgsv1.Money{AmountMinor: amount, Currency: "USD"},
			})
		case 3:
			_, _ = svc.TransferToAccount(ctx, &rgsv1.TransferToAccountRequest{
				Meta:      meta(accountID, rgsv1.ActorType_ACTOR_TYPE_PLAYER, idem),
				AccountId: accountID,
				Amount:    &rgsv1.Money{AmountMinor: amount, Currency: "USD"},
			})
		}

		bal, err := svc.GetBalance(ctx, &rgsv1.GetBalanceRequest{
			Meta:      meta(accountID, rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
			AccountId: accountID,
		})
		if err != nil {
			t.Fatalf("get balance at step %d: %v", i, err)
		}
		if bal.AvailableBalance.GetAmountMinor() < 0 {
			t.Fatalf("negative available balance at step %d: %d", i, bal.AvailableBalance.GetAmountMinor())
		}

		svc.mu.Lock()
		for txID, postings := range svc.postingsByTx {
			if !isBalanced(postings) {
				svc.mu.Unlock()
				t.Fatalf("unbalanced postings for tx=%s at step=%d", txID, i)
			}
		}
		svc.mu.Unlock()
	}
}
