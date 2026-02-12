package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
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
  identity_sessions,
  identity_lockouts,
  identity_credentials,
  remote_access_activity,
  system_window_events,
  promotional_awards,
  bonus_transactions,
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

func TestPostgresLedgerIdempotencyCleanupExpiredKeys(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	if _, err := db.Exec(`
INSERT INTO ledger_idempotency_keys (scope, idempotency_key, request_hash, response_payload, result_code, expires_at)
VALUES
  ('acct-cleanup|deposit', 'k-expired', '\x01', '{}'::jsonb, 'RESULT_CODE_OK', NOW() - INTERVAL '1 hour'),
  ('acct-cleanup|deposit', 'k-active', '\x02', '{}'::jsonb, 'RESULT_CODE_OK', NOW() + INTERVAL '1 hour')
`); err != nil {
		t.Fatalf("seed idempotency keys: %v", err)
	}

	svc := NewLedgerService(ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 45, 0, 0, time.UTC)}, db)
	deleted, err := svc.CleanupExpiredIdempotencyKeys(context.Background(), 100)
	if err != nil {
		t.Fatalf("cleanup expired keys err: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected deleted=1, got=%d", deleted)
	}

	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ledger_idempotency_keys`).Scan(&count); err != nil {
		t.Fatalf("count idempotency keys: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 remaining idempotency key, got=%d", count)
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

func TestPostgresIdentitySessionPersistenceAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	svcSeed := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 0, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour, db)
	respSet, err := svcSeed.SetCredential(context.Background(), &rgsv1.SetCredentialRequest{
		Meta:           meta("op-seed", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-sess-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "player-secret"),
		Reason:         "seed session user",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if respSet.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected set credential ok, got=%v", respSet.Meta.GetResultCode())
	}

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 10, 0, 0, time.UTC)}
	svcA := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour, db)
	login, err := svcA.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-sess-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-sess-1", Pin: "player-secret"},
		},
	})
	if err != nil {
		t.Fatalf("login err: %v", err)
	}
	if login.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected login ok, got=%v", login.Meta.GetResultCode())
	}

	svcB := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour, db)
	refreshed, err := svcB.RefreshToken(context.Background(), &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-sess-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: login.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("refresh err: %v", err)
	}
	if refreshed.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected refresh ok, got=%v", refreshed.Meta.GetResultCode())
	}

	svcC := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour, db)
	logout, err := svcC.Logout(context.Background(), &rgsv1.LogoutRequest{
		Meta:         meta("player-sess-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: refreshed.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("logout err: %v", err)
	}
	if logout.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected logout ok, got=%v", logout.Meta.GetResultCode())
	}

	denied, err := svcC.RefreshToken(context.Background(), &rgsv1.RefreshTokenRequest{
		Meta:         meta("player-sess-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		RefreshToken: refreshed.Token.GetRefreshToken(),
	})
	if err != nil {
		t.Fatalf("refresh after logout err: %v", err)
	}
	if denied.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied refresh after logout, got=%v", denied.Meta.GetResultCode())
	}
}

func TestPostgresIdentitySessionCleanupExpiredRows(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	if _, err := db.Exec(`
INSERT INTO identity_sessions (refresh_token, actor_id, actor_type, expires_at, revoked)
VALUES
  ('sess-expired', 'player-1', 'ACTOR_TYPE_PLAYER', NOW() - INTERVAL '1 hour', FALSE),
  ('sess-active', 'player-1', 'ACTOR_TYPE_PLAYER', NOW() + INTERVAL '1 hour', FALSE)
`); err != nil {
		t.Fatalf("seed identity sessions: %v", err)
	}

	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 20, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour, db)
	deleted, err := svc.CleanupExpiredSessions(context.Background(), 100)
	if err != nil {
		t.Fatalf("cleanup expired sessions err: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted session, got=%d", deleted)
	}
}

func TestPostgresIdentityHasActiveCredentials(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	svc := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 30, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour, db)
	ok, err := svc.HasActiveCredentials(context.Background())
	if err != nil {
		t.Fatalf("has active credentials err: %v", err)
	}
	if ok {
		t.Fatalf("expected no active credentials in clean database")
	}

	if _, err := db.Exec(`
INSERT INTO identity_credentials (actor_id, actor_type, password_hash, status)
VALUES ('op-bootstrap-1', 'ACTOR_TYPE_OPERATOR', '$2a$10$7jvnYQ5lzu4iAfDdc0AGJOhQJu1WDVYj1WFJsbgx5caX5/C/PObbW', 'active')
`); err != nil {
		t.Fatalf("seed bootstrap credential: %v", err)
	}
	ok, err = svc.HasActiveCredentials(context.Background())
	if err != nil {
		t.Fatalf("has active credentials after seed err: %v", err)
	}
	if !ok {
		t.Fatalf("expected active credentials after seed")
	}
}

func TestPostgresRemoteAccessActivityPersistenceAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 45, 0, 0, time.UTC)}
	guardA, err := NewRemoteAccessGuard(clk, audit.NewInMemoryStore(), []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new remote access guard err: %v", err)
	}
	guardA.SetDB(db)
	handler := guardA.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.test/v1/reporting/runs", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected trusted request to pass, got code=%d", rr.Code)
	}

	guardB, err := NewRemoteAccessGuard(clk, audit.NewInMemoryStore(), []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new remote access guard err: %v", err)
	}
	guardB.SetDB(db)
	auditSvc := NewAuditService(clk, guardB, audit.NewInMemoryStore())
	resp, err := auditSvc.ListRemoteAccessActivities(context.Background(), &rgsv1.ListRemoteAccessActivitiesRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list remote access activities err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok response, got=%v", resp.Meta.GetResultCode())
	}
	if len(resp.Activities) == 0 {
		t.Fatalf("expected persisted remote access activity row")
	}
}
