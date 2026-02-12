package server

import (
	"context"
	"fmt"
	"testing"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
)

func BenchmarkWageringPlaceWager(b *testing.B) {
	svc := NewWageringService(clock.RealClock{}, nil)
	ctx := context.Background()
	playerID := "player-bench"
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta := benchmarkMeta(playerID)
		meta.IdempotencyKey = fmt.Sprintf("wager-place-%d", i)
		_, err := svc.PlaceWager(ctx, &rgsv1.PlaceWagerRequest{
			Meta:     meta,
			PlayerId: playerID,
			GameId:   "game-bench",
			Stake: &rgsv1.Money{
				AmountMinor: 100,
				Currency:    "USD",
			},
		})
		if err != nil {
			b.Fatalf("place wager: %v", err)
		}
	}
}
