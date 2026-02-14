package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
)

func TestReportingGenerateSignificantEventsDTD(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)}
	eventsSvc := NewEventsService(clk)
	ledgerSvc := NewLedgerService(clk)
	reportingSvc := NewReportingService(clk, ledgerSvc, eventsSvc)

	ctx := context.Background()

	_, _ = eventsSvc.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:              "ev-old",
			EquipmentId:          "eq-1",
			EventCode:            "E1",
			OccurredAt:           "2026-02-11T10:00:00Z",
			LocalizedDescription: "old",
		},
	})
	_, _ = eventsSvc.SubmitSignificantEvent(ctx, &rgsv1.SubmitSignificantEventRequest{
		Meta: meta("svc-1", rgsv1.ActorType_ACTOR_TYPE_SERVICE, ""),
		Event: &rgsv1.SignificantEvent{
			EventId:              "ev-today",
			EquipmentId:          "eq-1",
			EventCode:            "E2",
			OccurredAt:           "2026-02-12T14:00:00Z",
			LocalizedDescription: "today",
		},
	})

	resp, err := reportingSvc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("generate report err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok result, got=%v", resp.Meta.GetResultCode())
	}

	var payload struct {
		RowCount int `json:"row_count"`
	}
	if err := json.Unmarshal(resp.ReportRun.Content, &payload); err != nil {
		t.Fatalf("unmarshal report content: %v", err)
	}
	if payload.RowCount != 1 {
		t.Fatalf("expected 1 row for DTD report, got=%d", payload.RowCount)
	}
	if resp.ReportRun.NoActivity {
		t.Fatalf("expected no_activity=false")
	}
}

func TestReportingNoActivityCSV(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)}
	reportingSvc := NewReportingService(clk, NewLedgerService(clk), NewEventsService(clk))

	resp, err := reportingSvc.GenerateReport(context.Background(), &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_CSV,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("generate report err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok result, got=%v", resp.Meta.GetResultCode())
	}
	if !resp.ReportRun.NoActivity {
		t.Fatalf("expected no_activity=true")
	}
	if !strings.Contains(string(resp.ReportRun.Content), "No Activity") {
		t.Fatalf("expected CSV to contain 'No Activity', content=%q", string(resp.ReportRun.Content))
	}
}

func TestReportingListAndGetRun(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 15, 0, 0, 0, time.UTC)}
	reportingSvc := NewReportingService(clk, NewLedgerService(clk), NewEventsService(clk))

	gen, _ := reportingSvc.GenerateReport(context.Background(), &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})

	list, err := reportingSvc.ListReportRuns(context.Background(), &rgsv1.ListReportRunsRequest{Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, "")})
	if err != nil {
		t.Fatalf("list runs err: %v", err)
	}
	if len(list.ReportRuns) == 0 {
		t.Fatalf("expected at least one report run")
	}

	got, err := reportingSvc.GetReportRun(context.Background(), &rgsv1.GetReportRunRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportRunId: gen.ReportRun.ReportRunId,
	})
	if err != nil {
		t.Fatalf("get run err: %v", err)
	}
	if got.ReportRun.ReportRunId != gen.ReportRun.ReportRunId {
		t.Fatalf("report run id mismatch")
	}
}

func TestReportingGenerateAccountTransactionStatement(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 16, 0, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	reportingSvc := NewReportingService(clk, ledgerSvc, NewEventsService(clk))
	ctx := context.Background()

	_, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-rpt-tx-1"),
		AccountId: "player-1",
		Amount:    &rgsv1.Money{AmountMinor: 500, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	resp, err := reportingSvc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("generate report err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok result, got=%v", resp.Meta.GetResultCode())
	}

	var payload struct {
		RowCount int `json:"row_count"`
	}
	if err := json.Unmarshal(resp.ReportRun.Content, &payload); err != nil {
		t.Fatalf("unmarshal report content: %v", err)
	}
	if payload.RowCount != 1 {
		t.Fatalf("expected one transaction row, got=%d", payload.RowCount)
	}
}

func TestReportingDisableInMemoryCacheDisablesFallbackAndRunRetention(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 16, 30, 0, 0, time.UTC)}
	ledgerSvc := NewLedgerService(clk)
	reportingSvc := NewReportingService(clk, ledgerSvc, NewEventsService(clk))
	reportingSvc.SetDisableInMemoryCache(true)
	ctx := context.Background()

	_, err := ledgerSvc.Deposit(ctx, &rgsv1.DepositRequest{
		Meta:      meta("player-1", rgsv1.ActorType_ACTOR_TYPE_PLAYER, "idem-rpt-disable-cache-1"),
		AccountId: "player-1",
		Amount:    &rgsv1.Money{AmountMinor: 500, Currency: "USD"},
	})
	if err != nil {
		t.Fatalf("deposit err: %v", err)
	}

	resp, err := reportingSvc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("generate report err: %v", err)
	}
	if resp.Meta.GetResultCode() != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("expected ok result, got=%v", resp.Meta.GetResultCode())
	}

	var payload struct {
		NoActivity bool `json:"no_activity"`
		RowCount   int  `json:"row_count"`
	}
	if err := json.Unmarshal(resp.ReportRun.Content, &payload); err != nil {
		t.Fatalf("unmarshal report content: %v", err)
	}
	if !payload.NoActivity || payload.RowCount != 0 {
		t.Fatalf("expected disabled-cache report payload to avoid in-memory fallback, got no_activity=%v row_count=%d", payload.NoActivity, payload.RowCount)
	}

	listResp, err := reportingSvc.ListReportRuns(ctx, &rgsv1.ListReportRunsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list report runs err: %v", err)
	}
	if len(listResp.ReportRuns) != 0 {
		t.Fatalf("expected no in-memory run retention when cache is disabled, got=%d", len(listResp.ReportRuns))
	}
}

func TestReportingActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 16, 45, 0, 0, time.UTC)}
	reportingSvc := NewReportingService(clk, NewLedgerService(clk), NewEventsService(clk))
	ctx := platformauth.WithActor(context.Background(), platformauth.Actor{ID: "ctx-op", Type: "ACTOR_TYPE_OPERATOR"})

	genResp, err := reportingSvc.GenerateReport(ctx, &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("generate mismatch err: %v", err)
	}
	if genResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied generate mismatch, got=%v", genResp.GetMeta().GetResultCode())
	}
	if genResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on generate, got=%q", genResp.GetMeta().GetDenialReason())
	}

	listResp, err := reportingSvc.ListReportRuns(ctx, &rgsv1.ListReportRunsRequest{
		Meta: meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
	})
	if err != nil {
		t.Fatalf("list mismatch err: %v", err)
	}
	if listResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list mismatch, got=%v", listResp.GetMeta().GetResultCode())
	}
	if listResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on list, got=%q", listResp.GetMeta().GetDenialReason())
	}

	seed, err := reportingSvc.GenerateReport(context.Background(), &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	})
	if err != nil {
		t.Fatalf("seed generate err: %v", err)
	}
	getResp, err := reportingSvc.GetReportRun(ctx, &rgsv1.GetReportRunRequest{
		Meta:        meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportRunId: seed.ReportRun.GetReportRunId(),
	})
	if err != nil {
		t.Fatalf("get mismatch err: %v", err)
	}
	if getResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied get mismatch, got=%v", getResp.GetMeta().GetResultCode())
	}
	if getResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on get, got=%q", getResp.GetMeta().GetDenialReason())
	}

	events := reportingSvc.AuditStore.Events()
	if len(events) == 0 {
		t.Fatalf("expected denied reporting audit events")
	}
	last := events[len(events)-1]
	if last.Action != "get_report_run" || last.Reason != "actor mismatch with token" {
		t.Fatalf("expected denied get_report_run audit with actor mismatch reason, got=%+v", last)
	}
}
