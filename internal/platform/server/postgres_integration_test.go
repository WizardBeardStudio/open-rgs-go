package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
)

func openPostgresIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("RGS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set RGS_TEST_DATABASE_URL to run postgres integration tests")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping postgres: %v", err)
	}
	return db
}

func resetPostgresIntegrationState(t *testing.T, db *sql.DB) {
	t.Helper()
	const q = `
TRUNCATE TABLE
  cashless_unresolved_transfers,
  ledger_postings,
  ledger_transactions,
  ledger_accounts,
  ledger_idempotency_keys,
  ingestion_buffer_audit,
  ingestion_buffers,
  meter_records,
  significant_events,
  equipment_registry,
  report_runs,
  config_current_values,
  config_changes,
  download_library_changes,
  audit_events
RESTART IDENTITY CASCADE
`
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("truncate integration tables: %v", err)
	}
}

func TestPostgresLedgerIdempotencyReplayAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewLedgerService(clk, db)
	firstDeposit, err := svcA.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-dep-1"),
		AccountId: "acct-pg-1",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("first deposit err: %v", err)
	}

	svcB := NewLedgerService(clk, db)
	replayedDeposit, err := svcB.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-dep-1"),
		AccountId: "acct-pg-1",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("replayed deposit err: %v", err)
	}
	if firstDeposit.Transaction.GetTransactionId() != replayedDeposit.Transaction.GetTransactionId() {
		t.Fatalf("deposit tx mismatch after restart: first=%s replay=%s", firstDeposit.Transaction.GetTransactionId(), replayedDeposit.Transaction.GetTransactionId())
	}

	firstWithdraw, err := svcB.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-wd-1"),
		AccountId: "acct-pg-1",
		Amount:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("first withdraw err: %v", err)
	}

	svcC := NewLedgerService(clk, db)
	replayedWithdraw, err := svcC.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-wd-1"),
		AccountId: "acct-pg-1",
		Amount:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("replayed withdraw err: %v", err)
	}
	if firstWithdraw.Transaction.GetTransactionId() != replayedWithdraw.Transaction.GetTransactionId() {
		t.Fatalf("withdraw tx mismatch after restart: first=%s replay=%s", firstWithdraw.Transaction.GetTransactionId(), replayedWithdraw.Transaction.GetTransactionId())
	}

	bal, err := svcC.GetBalance(ctx, &rgsv1.GetBalanceRequest{
		Meta:      meta("acct-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		AccountId: "acct-pg-1",
	})
	if err != nil {
		t.Fatalf("get balance err: %v", err)
	}
	if bal.AvailableBalance.GetAmountMinor() != 750 {
		t.Fatalf("expected balance 750 after replayed operations, got=%d", bal.AvailableBalance.GetAmountMinor())
	}
}

func TestPostgresLedgerIdempotencyRejectsMismatchedPayload(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 30, 0, 0, time.UTC)}
	ctx := context.Background()

	svc := NewLedgerService(clk, db)
	_, err := svc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-pg-mismatch", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-mismatch-1"),
		AccountId: "acct-pg-mismatch",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("initial deposit err: %v", err)
	}

	resp, err := svc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-pg-mismatch", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-mismatch-1"),
		AccountId: "acct-pg-mismatch",
		Amount:    &rgsv1.Money{AmountMinor: 1200, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("mismatched replay deposit err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid for mismatched idempotency replay, got=%v", resp.Meta.GetResultCode())
	}
}

func TestPostgresConfigApproveApplyAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewConfigService(clk, db)
	proposed, err := svcA.ProposeConfigChange(ctx, &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-a", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "security",
		ConfigKey:       "session_timeout",
		ProposedValue:   "600",
		Reason:          "tighten session policy",
	})
	if err != nil {
		t.Fatalf("propose err: %v", err)
	}

	svcB := NewConfigService(clk, db)
	approved, err := svcB.ApproveConfigChange(ctx, &rgsv1.ApproveConfigChangeRequest{
		Meta:     meta("op-b", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposed.Change.GetChangeId(),
		Reason:   "second operator approval",
	})
	if err != nil {
		t.Fatalf("approve err: %v", err)
	}
	if approved.Change.GetStatus() != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPROVED {
		t.Fatalf("expected approved status, got=%v", approved.Change.GetStatus())
	}

	svcC := NewConfigService(clk, db)
	applied, err := svcC.ApplyConfigChange(ctx, &rgsv1.ApplyConfigChangeRequest{
		Meta:     meta("op-c", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposed.Change.GetChangeId(),
		Reason:   "maintenance window",
	})
	if err != nil {
		t.Fatalf("apply err: %v", err)
	}
	if applied.Change.GetStatus() != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED {
		t.Fatalf("expected applied status, got=%v", applied.Change.GetStatus())
	}

	svcD := NewConfigService(clk, db)
	history, err := svcD.ListConfigHistory(ctx, &rgsv1.ListConfigHistoryRequest{
		Meta:                  meta("op-a", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespaceFilter: "security",
	})
	if err != nil {
		t.Fatalf("list history err: %v", err)
	}
	if len(history.Changes) != 1 {
		t.Fatalf("expected 1 config history item, got=%d", len(history.Changes))
	}
	if history.Changes[0].GetStatus() != rgsv1.ConfigChangeStatus_CONFIG_CHANGE_STATUS_APPLIED {
		t.Fatalf("expected persisted applied status, got=%v", history.Changes[0].GetStatus())
	}
}

func TestPostgresReportingPayloadsFromDatabase(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	if _, err := db.Exec(`
INSERT INTO equipment_registry (equipment_id, status, location)
VALUES ('eq-pg-1', 'active', 'lab')
`); err != nil {
		t.Fatalf("seed equipment: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO significant_events (
  event_id, equipment_id, event_code, localized_description, severity,
  occurred_at, received_at, recorded_at
) VALUES (
  'ev-pg-1', 'eq-pg-1', 'E100', 'door open', 'EVENT_SEVERITY_WARN',
  '2026-02-13T09:30:00Z', '2026-02-13T09:30:01Z', '2026-02-13T09:30:02Z'
)
`); err != nil {
		t.Fatalf("seed significant event: %v", err)
	}
	if _, err := db.Exec(`
INSERT INTO ledger_accounts (account_id, player_id, account_type, status, currency_code, available_balance_minor, pending_balance_minor)
VALUES ('acct-pg-rpt-1', 'acct-pg-rpt-1', 'player_cashless', 'active', 'USD', 900, 100)
`); err != nil {
		t.Fatalf("seed ledger account: %v", err)
	}

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 12, 0, 0, 0, time.UTC)}
	svc := NewReportingService(clk, nil, nil, db)
	ctx := context.Background()

	eventsReport, err := svc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-r1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-pg",
	})
	if err != nil {
		t.Fatalf("generate events report err: %v", err)
	}
	var eventPayload struct {
		RowCount int `json:"row_count"`
	}
	if err := json.Unmarshal(eventsReport.ReportRun.GetContent(), &eventPayload); err != nil {
		t.Fatalf("decode events payload: %v", err)
	}
	if eventPayload.RowCount != 1 {
		t.Fatalf("expected 1 significant event row from DB, got=%d", eventPayload.RowCount)
	}

	cashlessReport, err := svc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-r1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-pg",
	})
	if err != nil {
		t.Fatalf("generate cashless report err: %v", err)
	}
	var cashlessPayload struct {
		RowCount       int   `json:"row_count"`
		TotalAvailable int64 `json:"total_available"`
		TotalPending   int64 `json:"total_pending"`
	}
	if err := json.Unmarshal(cashlessReport.ReportRun.GetContent(), &cashlessPayload); err != nil {
		t.Fatalf("decode cashless payload: %v", err)
	}
	if cashlessPayload.RowCount != 1 {
		t.Fatalf("expected 1 cashless row from DB, got=%d", cashlessPayload.RowCount)
	}
	if cashlessPayload.TotalAvailable != 900 || cashlessPayload.TotalPending != 100 {
		t.Fatalf("unexpected cashless totals: available=%d pending=%d", cashlessPayload.TotalAvailable, cashlessPayload.TotalPending)
	}
}
