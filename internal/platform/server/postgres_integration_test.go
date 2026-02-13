package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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
  wagering_idempotency_keys,
  wagers,
  cashless_unresolved_transfers,
  ledger_postings,
  ledger_transactions,
  ledger_accounts,
  ledger_eft_lockouts,
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
  identity_login_rate_limits,
  identity_lockouts,
  identity_credentials,
  player_sessions,
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

func TestPostgresLedgerMutationAfterRestartUsesDurableBalance(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 10, 15, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewLedgerService(clk, db)
	_, err := svcA.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-pg-mutate", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-mut-dep-1"),
		AccountId: "acct-pg-mutate",
		Amount:    &rgsv1.Money{AmountMinor: 1000, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("seed deposit err: %v", err)
	}

	svcB := NewLedgerService(clk, db)
	withdraw, err := svcB.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-pg-mutate", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-mut-wd-1"),
		AccountId: "acct-pg-mutate",
		Amount:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("withdraw after restart err: %v", err)
	}
	if withdraw.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected successful withdraw after restart, got=%v reason=%q", withdraw.Meta.GetResultCode(), withdraw.Meta.GetDenialReason())
	}
	if withdraw.AvailableBalance.GetAmountMinor() != 750 {
		t.Fatalf("expected remaining balance 750, got=%d", withdraw.AvailableBalance.GetAmountMinor())
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

func TestPostgresSessionsPersistAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 30, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewSessionsService(clk, db)
	start, err := svcA.StartSession(ctx, &rgsv1.StartSessionRequest{
		Meta:     meta("player-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		PlayerId: "player-pg-1",
		DeviceId: "device-pg-a",
	})
	if err != nil {
		t.Fatalf("start session err: %v", err)
	}
	if start.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("start session code=%v reason=%q", start.Meta.GetResultCode(), start.Meta.GetDenialReason())
	}

	svcB := NewSessionsService(clk, db)
	got, err := svcB.GetSession(ctx, &rgsv1.GetSessionRequest{
		Meta:      meta("player-pg-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		SessionId: start.Session.GetSessionId(),
	})
	if err != nil {
		t.Fatalf("get session err: %v", err)
	}
	if got.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("get session code=%v reason=%q", got.Meta.GetResultCode(), got.Meta.GetDenialReason())
	}
	if got.Session.GetSessionId() != start.Session.GetSessionId() {
		t.Fatalf("session mismatch after restart: got=%s want=%s", got.Session.GetSessionId(), start.Session.GetSessionId())
	}
	if got.Session.GetState() != rgsv1.SessionState_SESSION_STATE_ACTIVE {
		t.Fatalf("expected active session after restart, got=%v", got.Session.GetState())
	}
}

func TestPostgresAuditServiceListsPersistedAuditEvents(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 45, 0, 0, time.UTC)}
	ctx := context.Background()

	ledgerSvc := NewLedgerService(clk, db)
	_, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-audit-pg", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-pg-1"),
		AccountId: "acct-audit-pg",
		Amount:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	auditSvc := NewAuditService(clk, nil)
	auditSvc.SetDB(db)
	resp, err := auditSvc.ListAuditEvents(ctx, &rgsv1.ListAuditEventsRequest{
		Meta:             meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ObjectTypeFilter: "ledger_account",
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("unexpected list audit result: code=%v reason=%q", resp.Meta.GetResultCode(), resp.Meta.GetDenialReason())
	}
	if len(resp.Events) == 0 {
		t.Fatalf("expected persisted audit events from db")
	}
}

func TestPostgresAuditServiceListsPersistedConfigAuditEvents(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 50, 0, 0, time.UTC)}
	ctx := context.Background()

	configSvc := NewConfigService(clk, db)
	_, err := configSvc.ProposeConfigChange(ctx, &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "ops",
		ConfigKey:       "max_sessions",
		ProposedValue:   "5",
		Reason:          "integration-test",
	})
	if err != nil {
		t.Fatalf("propose config change err: %v", err)
	}

	auditSvc := NewAuditService(clk, nil)
	auditSvc.SetDB(db)
	resp, err := auditSvc.ListAuditEvents(ctx, &rgsv1.ListAuditEventsRequest{
		Meta:             meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ObjectTypeFilter: "config_change",
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("unexpected list audit result: code=%v reason=%q", resp.Meta.GetResultCode(), resp.Meta.GetDenialReason())
	}
	if len(resp.Events) == 0 {
		t.Fatalf("expected persisted config audit events from db")
	}
}

func TestPostgresAuditChainVerificationPassesForPersistedEvents(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 12, 55, 0, 0, time.UTC)}
	ctx := context.Background()

	ledgerSvc := NewLedgerService(clk, db)
	if _, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-audit-chain", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-chain-1"),
		AccountId: "acct-audit-chain",
		Amount:    &rgsv1.Money{AmountMinor: 500, Currency: "USD"},
	}); err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	configSvc := NewConfigService(clk, db)
	if _, err := configSvc.ProposeConfigChange(ctx, &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "ops",
		ConfigKey:       "audit_chain_verify",
		ProposedValue:   "enabled",
		Reason:          "test",
	}); err != nil {
		t.Fatalf("propose config change err: %v", err)
	}

	if err := verifyAuditChainFromDB(ctx, db, clk.now.Format("2006-01-02")); err != nil {
		t.Fatalf("verify audit chain err: %v", err)
	}
}

func TestPostgresAuditChainVerificationDetectsTamper(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 13, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	ledgerSvc := NewLedgerService(clk, db)
	if _, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-audit-chain-bad", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-chain-bad-1"),
		AccountId: "acct-audit-chain-bad",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	}); err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	partitionDay := clk.now.Format("2006-01-02")
	_, err := db.ExecContext(ctx, `
INSERT INTO audit_events (
  audit_id, occurred_at, recorded_at, actor_id, actor_type, auth_context,
  object_type, object_id, action, before_state, after_state, result, reason,
  partition_day, hash_prev, hash_curr
)
VALUES (
  'tampered-audit-row', $1::timestamptz, $1::timestamptz, 'tamper', 'service', '{}'::jsonb,
  'tamper_object', 'tamper_id', 'tamper_action', '{}'::jsonb, '{}'::jsonb, 'success', 'tamper',
  $2::date, 'bad-prev', 'bad-curr'
)
`, clk.now.Add(1*time.Second).Format(time.RFC3339Nano), partitionDay)
	if err != nil {
		t.Fatalf("insert tampered audit row err: %v", err)
	}

	if err := verifyAuditChainFromDB(ctx, db, partitionDay); err == nil {
		t.Fatalf("expected audit chain verification to detect tamper")
	}
}

func TestPostgresAuditServiceVerifyAuditChain(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 13, 5, 0, 0, time.UTC)}
	ctx := context.Background()

	ledgerSvc := NewLedgerService(clk, db)
	if _, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-audit-verify-api", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-verify-api-1"),
		AccountId: "acct-audit-verify-api",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	}); err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	auditSvc := NewAuditService(clk, nil)
	auditSvc.SetDB(db)
	resp, err := auditSvc.VerifyAuditChain(ctx, &rgsv1.VerifyAuditChainRequest{
		Meta:         meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PartitionDay: clk.now.Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("verify audit chain err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK || !resp.Valid {
		t.Fatalf("expected valid chain, got code=%v valid=%v", resp.Meta.GetResultCode(), resp.Valid)
	}
}

func TestPostgresAuditServiceRejectsInvalidPageToken(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 17, 13, 10, 0, 0, time.UTC)}
	ctx := context.Background()

	ledgerSvc := NewLedgerService(clk, db)
	if _, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-audit-bad-token", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-audit-bad-token-1"),
		AccountId: "acct-audit-bad-token",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	}); err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	auditSvc := NewAuditService(clk, nil)
	auditSvc.SetDB(db)
	resp, err := auditSvc.ListAuditEvents(ctx, &rgsv1.ListAuditEventsRequest{
		Meta:      meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		PageToken: "bad-token",
	})
	if err != nil {
		t.Fatalf("list audit events err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_INVALID {
		t.Fatalf("expected invalid for invalid db page token, got=%v", resp.Meta.GetResultCode())
	}
}

func TestPostgresLedgerEFTLockoutPersistsAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 11, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewLedgerService(clk, db)
	svcA.SetEFTFraudPolicy(2, 15*time.Minute)
	_, err := svcA.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("acct-eft-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-eft-dep-1"),
		AccountId: "acct-eft-1",
		Amount:    &rgsv1.Money{AmountMinor: 100, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("seed deposit err: %v", err)
	}

	for i := 0; i < 2; i++ {
		resp, err := svcA.Withdraw(ctx, &rgsv1.WithdrawRequest{
			Meta:      meta("acct-eft-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-eft-wd-"+strconv.Itoa(i+1)),
			AccountId: "acct-eft-1",
			Amount:    &rgsv1.Money{AmountMinor: 200, Currency: "USD"},
		})
		if err != nil {
			t.Fatalf("withdraw %d err: %v", i+1, err)
		}
		if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
			t.Fatalf("expected denied withdraw %d, got=%v", i+1, resp.Meta.GetResultCode())
		}
	}

	svcB := NewLedgerService(clk, db)
	svcB.SetEFTFraudPolicy(2, 15*time.Minute)
	locked, err := svcB.Withdraw(ctx, &rgsv1.WithdrawRequest{
		Meta:      meta("acct-eft-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-eft-wd-3"),
		AccountId: "acct-eft-1",
		Amount:    &rgsv1.Money{AmountMinor: 50, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("withdraw after restart err: %v", err)
	}
	if locked.Meta.GetDenialReason() != "eft account locked" {
		t.Fatalf("expected eft account locked denial, got=%q", locked.Meta.GetDenialReason())
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

func TestPostgresIdentityLoginRateLimitAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	svcSeed := NewIdentityService(ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 40, 0, 0, time.UTC)}, "test-secret", 15*time.Minute, time.Hour, db)
	respSet, err := svcSeed.SetCredential(context.Background(), &rgsv1.SetCredentialRequest{
		Meta:           meta("op-seed", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		Actor:          &rgsv1.Actor{ActorId: "player-rate-1", ActorType: rgsv1.ActorType_ACTOR_TYPE_PLAYER},
		CredentialHash: mustBcryptHash(t, "player-secret"),
		Reason:         "seed rate limit user",
	})
	if err != nil {
		t.Fatalf("set credential err: %v", err)
	}
	if respSet.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected set credential ok, got=%v", respSet.Meta.GetResultCode())
	}

	clk := ledgerFixedClock{now: time.Date(2026, 2, 13, 15, 41, 0, 0, time.UTC)}
	svcA := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour, db)
	svcA.SetLoginRateLimit(2, time.Minute)

	for i := 0; i < 3; i++ {
		resp, err := svcA.Login(context.Background(), &rgsv1.LoginRequest{
			Meta: meta("player-rate-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
			Credentials: &rgsv1.LoginRequest_Player{
				Player: &rgsv1.PlayerCredentials{PlayerId: "player-rate-1", Pin: "wrong-secret"},
			},
		})
		if err != nil {
			t.Fatalf("login attempt %d err: %v", i+1, err)
		}
		if i < 2 && resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
			t.Fatalf("expected denied credentials on attempt %d, got=%v", i+1, resp.Meta.GetResultCode())
		}
		if i == 2 && resp.Meta.GetDenialReason() != "rate limit exceeded" {
			t.Fatalf("expected rate limit exceeded on third attempt, got=%q", resp.Meta.GetDenialReason())
		}
	}

	svcB := NewIdentityService(clk, "test-secret", 15*time.Minute, time.Hour, db)
	svcB.SetLoginRateLimit(2, time.Minute)
	resp, err := svcB.Login(context.Background(), &rgsv1.LoginRequest{
		Meta: meta("player-rate-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, ""),
		Credentials: &rgsv1.LoginRequest_Player{
			Player: &rgsv1.PlayerCredentials{PlayerId: "player-rate-1", Pin: "wrong-secret"},
		},
	})
	if err != nil {
		t.Fatalf("login after restart err: %v", err)
	}
	if resp.Meta.GetDenialReason() != "rate limit exceeded" {
		t.Fatalf("expected persisted rate limit denial after restart, got=%q", resp.Meta.GetDenialReason())
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
	auditSvc.SetDB(db)
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

	auditEventsResp, err := auditSvc.ListAuditEvents(context.Background(), &rgsv1.ListAuditEventsRequest{
		Meta:             meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ObjectTypeFilter: "remote_access",
	})
	if err != nil {
		t.Fatalf("list remote access audit events err: %v", err)
	}
	if auditEventsResp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok audit events response, got=%v", auditEventsResp.Meta.GetResultCode())
	}
	if len(auditEventsResp.Events) == 0 {
		t.Fatalf("expected persisted remote access audit event row")
	}
}

func TestPostgresWageringPersistenceAcrossRestart(t *testing.T) {
	db := openPostgresIntegrationDB(t)
	resetPostgresIntegrationState(t, db)

	clk := ledgerFixedClock{now: time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)}
	ctx := context.Background()

	svcA := NewWageringService(clk, db)
	placeReq := &rgsv1.PlaceWagerRequest{
		Meta:     meta("player-w-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-pg-wager-place-1"),
		PlayerId: "player-w-1",
		GameId:   "game-1",
		Stake:    &rgsv1.Money{AmountMinor: 250, Currency: "USD"},
	}
	placedA, err := svcA.PlaceWager(ctx, placeReq)
	if err != nil {
		t.Fatalf("place wager err: %v", err)
	}
	if placedA.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected place wager ok, got=%v", placedA.Meta.GetResultCode())
	}

	svcB := NewWageringService(clk, db)
	placedB, err := svcB.PlaceWager(ctx, placeReq)
	if err != nil {
		t.Fatalf("replay place wager err: %v", err)
	}
	if placedA.Wager.GetWagerId() != placedB.Wager.GetWagerId() {
		t.Fatalf("expected idempotent wager id across restart, got=%s vs %s", placedA.Wager.GetWagerId(), placedB.Wager.GetWagerId())
	}

	settleReq := &rgsv1.SettleWagerRequest{
		Meta:       meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, "idem-pg-wager-settle-1"),
		WagerId:    placedA.Wager.GetWagerId(),
		Payout:     &rgsv1.Money{AmountMinor: 400, Currency: "USD"},
		OutcomeRef: "outcome-pg-1",
	}
	settled, err := svcB.SettleWager(ctx, settleReq)
	if err != nil {
		t.Fatalf("settle wager err: %v", err)
	}
	if settled.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected settle wager ok, got=%v", settled.Meta.GetResultCode())
	}

	svcC := NewWageringService(clk, db)
	replayedSettle, err := svcC.SettleWager(ctx, settleReq)
	if err != nil {
		t.Fatalf("replay settle wager err: %v", err)
	}
	if replayedSettle.Wager.GetStatus() != rgsv1.WagerStatus_WAGER_STATUS_SETTLED {
		t.Fatalf("expected settled status after replay, got=%v", replayedSettle.Wager.GetStatus())
	}
}
