package server

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeardstudio/open-rgs-go/internal/platform/clock"
)

func openPostgresBenchmarkDB(b *testing.B) *sql.DB {
	b.Helper()
	dsn := os.Getenv("RGS_TEST_DATABASE_URL")
	if dsn == "" {
		b.Skip("set RGS_TEST_DATABASE_URL to run postgres benchmarks")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		b.Fatalf("open postgres: %v", err)
	}
	b.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		b.Fatalf("ping postgres: %v", err)
	}
	return db
}

func resetPostgresBenchmarkState(b *testing.B, db *sql.DB) {
	b.Helper()
	const q = `
TRUNCATE TABLE
  wagering_idempotency_keys,
  wagers,
  cashless_unresolved_transfers,
  ledger_postings,
  ledger_transactions,
  ledger_accounts,
  ledger_eft_lockouts,
  ledger_idempotency_keys,
  player_sessions,
  audit_events
RESTART IDENTITY CASCADE
`
	if _, err := db.Exec(q); err != nil {
		b.Fatalf("truncate benchmark tables: %v", err)
	}
}

func BenchmarkLedgerDepositPostgres(b *testing.B) {
	db := openPostgresBenchmarkDB(b)
	resetPostgresBenchmarkState(b, db)

	svc := NewLedgerService(clock.RealClock{}, db)
	svc.SetDisableInMemoryIdempotencyCache(true)

	ctx := context.Background()
	accountID := "acct-bench-pg"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta := benchmarkMeta(accountID)
		meta.IdempotencyKey = fmt.Sprintf("dep-pg-%d", i)
		_, err := svc.Deposit(ctx, &rgsv1.DepositRequest{
			Meta:      meta,
			AccountId: accountID,
			Amount: &rgsv1.Money{
				AmountMinor: 1,
				Currency:    "USD",
			},
		})
		if err != nil {
			b.Fatalf("deposit postgres: %v", err)
		}
	}
}

func BenchmarkWageringPlaceWagerPostgres(b *testing.B) {
	db := openPostgresBenchmarkDB(b)
	resetPostgresBenchmarkState(b, db)

	svc := NewWageringService(clock.RealClock{}, db)
	svc.SetDisableInMemoryIdempotencyCache(true)

	ctx := context.Background()
	playerID := "player-bench-pg"

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		meta := benchmarkMeta(playerID)
		meta.IdempotencyKey = fmt.Sprintf("wager-place-pg-%d", i)
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
			b.Fatalf("place wager postgres: %v", err)
		}
	}
}
