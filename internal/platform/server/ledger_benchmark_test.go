package server

import (
	"context"
	"fmt"
	"testing"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/clock"
)

func benchmarkMeta(actorID string) *rgsv1.RequestMeta {
	return &rgsv1.RequestMeta{
		RequestId: fmt.Sprintf("bench-%s", actorID),
		Actor: &rgsv1.Actor{
			ActorId:   actorID,
			ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER,
		},
		Source: &rgsv1.Source{
			Ip: "127.0.0.1",
		},
	}
}

func BenchmarkLedgerDeposit(b *testing.B) {
	svc := NewLedgerService(clock.RealClock{}, nil)
	ctx := context.Background()
	accountID := "acct-bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta := benchmarkMeta(accountID)
		meta.IdempotencyKey = fmt.Sprintf("dep-%d", i)
		_, err := svc.Deposit(ctx, &rgsv1.DepositRequest{
			Meta:      meta,
			AccountId: accountID,
			Amount: &rgsv1.Money{
				AmountMinor: 1,
				Currency:    "USD",
			},
		})
		if err != nil {
			b.Fatalf("deposit: %v", err)
		}
	}
}
