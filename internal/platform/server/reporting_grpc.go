package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"sort"
	"strconv"
	"sync"
	"time"

	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"github.com/wizardbeard/open-rgs-go/internal/platform/audit"
	"github.com/wizardbeard/open-rgs-go/internal/platform/clock"
	"google.golang.org/protobuf/proto"
)

type ReportingService struct {
	rgsv1.UnimplementedReportingServiceServer

	Clock      clock.Clock
	AuditStore *audit.InMemoryStore

	Ledger *LedgerService
	Events *EventsService

	mu          sync.Mutex
	runs        map[string]*rgsv1.ReportRun
	runOrder    []string
	nextRunID   int64
	nextAuditID int64
	db          *sql.DB
}

func NewReportingService(clk clock.Clock, ledger *LedgerService, events *EventsService, db ...*sql.DB) *ReportingService {
	var handle *sql.DB
	if len(db) > 0 {
		handle = db[0]
	}
	return &ReportingService{
		Clock:      clk,
		AuditStore: audit.NewInMemoryStore(),
		Ledger:     ledger,
		Events:     events,
		runs:       make(map[string]*rgsv1.ReportRun),
		db:         handle,
	}
}

func (s *ReportingService) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now().UTC()
}

func (s *ReportingService) responseMeta(meta *rgsv1.RequestMeta, code rgsv1.ResultCode, denial string) *rgsv1.ResponseMeta {
	return &rgsv1.ResponseMeta{
		RequestId:    requestID(meta),
		ResultCode:   code,
		DenialReason: denial,
		ServerTime:   s.now().Format(time.RFC3339Nano),
	}
}

func (s *ReportingService) authorize(ctx context.Context, meta *rgsv1.RequestMeta) (bool, string) {
	actor, reason := resolveActor(ctx, meta)
	if reason != "" {
		return false, reason
	}
	switch actor.ActorType {
	case rgsv1.ActorType_ACTOR_TYPE_OPERATOR, rgsv1.ActorType_ACTOR_TYPE_SERVICE:
		return true, ""
	default:
		return false, "unauthorized actor type"
	}
}

func (s *ReportingService) nextRunIDLocked() string {
	s.nextRunID++
	return "report-" + strconv.FormatInt(s.nextRunID, 10)
}

func (s *ReportingService) nextAuditIDLocked() string {
	s.nextAuditID++
	return "reporting-audit-" + strconv.FormatInt(s.nextAuditID, 10)
}

func (s *ReportingService) appendAudit(meta *rgsv1.RequestMeta, objectID, action string, before, after []byte, result audit.Result, reason string) error {
	if s.AuditStore == nil {
		return audit.ErrCorruptChain
	}
	actorID := "system"
	actorType := "service"
	if meta != nil && meta.Actor != nil {
		actorID = meta.Actor.ActorId
		actorType = meta.Actor.ActorType.String()
	}
	now := s.now()
	_, err := s.AuditStore.Append(audit.Event{
		AuditID:      s.nextAuditIDLocked(),
		OccurredAt:   now,
		RecordedAt:   now,
		ActorID:      actorID,
		ActorType:    actorType,
		ObjectType:   "report_run",
		ObjectID:     objectID,
		Action:       action,
		Before:       before,
		After:        after,
		Result:       result,
		Reason:       reason,
		PartitionDay: now.Format("2006-01-02"),
	})
	return err
}

func cloneRun(in *rgsv1.ReportRun) *rgsv1.ReportRun {
	if in == nil {
		return nil
	}
	cp, _ := proto.Clone(in).(*rgsv1.ReportRun)
	return cp
}

func intervalStart(now time.Time, interval rgsv1.ReportInterval) time.Time {
	now = now.UTC()
	switch interval {
	case rgsv1.ReportInterval_REPORT_INTERVAL_DTD:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case rgsv1.ReportInterval_REPORT_INTERVAL_MTD:
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	case rgsv1.ReportInterval_REPORT_INTERVAL_YTD:
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	case rgsv1.ReportInterval_REPORT_INTERVAL_LTD:
		return time.Time{}
	default:
		return time.Time{}
	}
}

func inInterval(ts time.Time, interval rgsv1.ReportInterval, now time.Time) bool {
	if interval == rgsv1.ReportInterval_REPORT_INTERVAL_LTD {
		return true
	}
	start := intervalStart(now, interval)
	if ts.IsZero() {
		return false
	}
	return !ts.Before(start) && !ts.After(now)
}

func parseTS(v string) time.Time {
	if v == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func reportTitle(t rgsv1.ReportType) string {
	switch t {
	case rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS:
		return "System Significant Events and Alterations"
	case rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY:
		return "Cashless Liability Summary"
	case rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT:
		return "Account Transaction Statement"
	default:
		return "Unknown Report"
	}
}

func (s *ReportingService) buildSignificantEventsPayload(interval rgsv1.ReportInterval, operatorID string) (map[string]any, bool) {
	now := s.now()
	rows := make([]map[string]any, 0)
	if s.db != nil {
		dbRows, err := s.fetchSignificantEventsRows(now, interval)
		if err == nil {
			rows = dbRows
		}
	}
	if len(rows) == 0 && s.Events != nil {
		s.Events.mu.Lock()
		ids := append([]string(nil), s.Events.eventOrder...)
		for _, id := range ids {
			e := s.Events.events[id]
			if e == nil {
				continue
			}
			ts := parseTS(e.OccurredAt)
			if ts.IsZero() {
				ts = parseTS(e.RecordedAt)
			}
			if !inInterval(ts, interval, now) {
				continue
			}
			rows = append(rows, map[string]any{
				"event_id":              e.EventId,
				"equipment_id":          e.EquipmentId,
				"event_code":            e.EventCode,
				"localized_description": e.LocalizedDescription,
				"severity":              e.Severity.String(),
				"occurred_at":           e.OccurredAt,
				"received_at":           e.ReceivedAt,
				"recorded_at":           e.RecordedAt,
			})
		}
		s.Events.mu.Unlock()
	}
	noActivity := len(rows) == 0
	payload := map[string]any{
		"operator_id":       operatorID,
		"report_title":      reportTitle(rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS),
		"selected_interval": interval.String(),
		"generated_at":      now.Format(time.RFC3339Nano),
		"no_activity":       noActivity,
		"row_count":         len(rows),
		"rows":              rows,
	}
	if noActivity {
		payload["note"] = "No Activity"
	}
	return payload, noActivity
}

func (s *ReportingService) buildCashlessLiabilityPayload(interval rgsv1.ReportInterval, operatorID string) (map[string]any, bool) {
	now := s.now()
	rows := make([]map[string]any, 0)
	var totalAvailable int64
	var totalPending int64

	if s.db != nil {
		dbRows, avail, pending, err := s.fetchCashlessLiabilityRows()
		if err == nil {
			rows = dbRows
			totalAvailable = avail
			totalPending = pending
		}
	}

	if len(rows) == 0 && s.Ledger != nil {
		s.Ledger.mu.Lock()
		ids := make([]string, 0, len(s.Ledger.accounts))
		for id := range s.Ledger.accounts {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			acct := s.Ledger.accounts[id]
			if acct == nil {
				continue
			}
			rows = append(rows, map[string]any{
				"account_id": id,
				"currency":   acct.currency,
				"available":  acct.available,
				"pending":    acct.pending,
				"total":      acct.available + acct.pending,
			})
			totalAvailable += acct.available
			totalPending += acct.pending
		}
		s.Ledger.mu.Unlock()
	}

	noActivity := len(rows) == 0
	payload := map[string]any{
		"operator_id":       operatorID,
		"report_title":      reportTitle(rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY),
		"selected_interval": interval.String(),
		"generated_at":      now.Format(time.RFC3339Nano),
		"no_activity":       noActivity,
		"row_count":         len(rows),
		"total_available":   totalAvailable,
		"total_pending":     totalPending,
		"rows":              rows,
	}
	if noActivity {
		payload["note"] = "No Activity"
	}
	return payload, noActivity
}

func (s *ReportingService) buildAccountTransactionStatementPayload(interval rgsv1.ReportInterval, operatorID string) (map[string]any, bool) {
	now := s.now()
	rows := make([]map[string]any, 0)

	if s.db != nil {
		dbRows, err := s.fetchAccountTransactionStatementRows(now, interval)
		if err == nil {
			rows = dbRows
		}
	}

	if len(rows) == 0 && s.Ledger != nil {
		s.Ledger.mu.Lock()
		accountIDs := make([]string, 0, len(s.Ledger.transactionsByAcct))
		for accountID := range s.Ledger.transactionsByAcct {
			accountIDs = append(accountIDs, accountID)
		}
		sort.Strings(accountIDs)
		for _, accountID := range accountIDs {
			txs := s.Ledger.transactionsByAcct[accountID]
			for _, tx := range txs {
				if tx == nil {
					continue
				}
				ts := parseTS(tx.OccurredAt)
				if !inInterval(ts, interval, now) {
					continue
				}
				rows = append(rows, map[string]any{
					"transaction_id":   tx.TransactionId,
					"account_id":       tx.AccountId,
					"transaction_type": tx.TransactionType.String(),
					"amount_minor":     tx.Amount.GetAmountMinor(),
					"currency":         tx.Amount.GetCurrency(),
					"occurred_at":      tx.OccurredAt,
					"authorization_id": tx.AuthorizationId,
				})
			}
		}
		s.Ledger.mu.Unlock()
	}

	noActivity := len(rows) == 0
	payload := map[string]any{
		"operator_id":       operatorID,
		"report_title":      reportTitle(rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT),
		"selected_interval": interval.String(),
		"generated_at":      now.Format(time.RFC3339Nano),
		"no_activity":       noActivity,
		"row_count":         len(rows),
		"rows":              rows,
	}
	if noActivity {
		payload["note"] = "No Activity"
	}
	return payload, noActivity
}

func payloadToCSV(reportType rgsv1.ReportType, payload map[string]any) ([]byte, error) {
	buf := &bytes.Buffer{}
	w := csv.NewWriter(buf)

	switch reportType {
	case rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS:
		_ = w.Write([]string{"operator_id", "report_title", "selected_interval", "generated_at"})
		_ = w.Write([]string{toString(payload["operator_id"]), toString(payload["report_title"]), toString(payload["selected_interval"]), toString(payload["generated_at"])})
		_ = w.Write([]string{"event_id", "equipment_id", "event_code", "localized_description", "severity", "occurred_at", "received_at", "recorded_at"})
		rows, _ := payload["rows"].([]map[string]any)
		if len(rows) == 0 {
			_ = w.Write([]string{"No Activity"})
		}
		for _, r := range rows {
			_ = w.Write([]string{toString(r["event_id"]), toString(r["equipment_id"]), toString(r["event_code"]), toString(r["localized_description"]), toString(r["severity"]), toString(r["occurred_at"]), toString(r["received_at"]), toString(r["recorded_at"])})
		}
	case rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY:
		_ = w.Write([]string{"operator_id", "report_title", "selected_interval", "generated_at", "total_available", "total_pending"})
		_ = w.Write([]string{toString(payload["operator_id"]), toString(payload["report_title"]), toString(payload["selected_interval"]), toString(payload["generated_at"]), toString(payload["total_available"]), toString(payload["total_pending"])})
		_ = w.Write([]string{"account_id", "currency", "available", "pending", "total"})
		rows, _ := payload["rows"].([]map[string]any)
		if len(rows) == 0 {
			_ = w.Write([]string{"No Activity"})
		}
		for _, r := range rows {
			_ = w.Write([]string{toString(r["account_id"]), toString(r["currency"]), toString(r["available"]), toString(r["pending"]), toString(r["total"])})
		}
	case rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT:
		_ = w.Write([]string{"operator_id", "report_title", "selected_interval", "generated_at"})
		_ = w.Write([]string{toString(payload["operator_id"]), toString(payload["report_title"]), toString(payload["selected_interval"]), toString(payload["generated_at"])})
		_ = w.Write([]string{"transaction_id", "account_id", "transaction_type", "amount_minor", "currency", "occurred_at", "authorization_id"})
		rows, _ := payload["rows"].([]map[string]any)
		if len(rows) == 0 {
			_ = w.Write([]string{"No Activity"})
		}
		for _, r := range rows {
			_ = w.Write([]string{toString(r["transaction_id"]), toString(r["account_id"]), toString(r["transaction_type"]), toString(r["amount_minor"]), toString(r["currency"]), toString(r["occurred_at"]), toString(r["authorization_id"])})
		}
	default:
		_ = w.Write([]string{"No Activity"})
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(x)
		if string(b) == "null" {
			return ""
		}
		return string(b)
	}
}

func (s *ReportingService) GenerateReport(ctx context.Context, req *rgsv1.GenerateReportRequest) (*rgsv1.GenerateReportResponse, error) {
	if req == nil {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "request is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "", "generate_report", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}
	if req.ReportType == rgsv1.ReportType_REPORT_TYPE_UNSPECIFIED {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "report_type is required")}, nil
	}
	if req.Interval == rgsv1.ReportInterval_REPORT_INTERVAL_UNSPECIFIED {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "interval is required")}, nil
	}
	if req.Format == rgsv1.ReportFormat_REPORT_FORMAT_UNSPECIFIED {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "format is required")}, nil
	}

	var payload map[string]any
	var noActivity bool
	switch req.ReportType {
	case rgsv1.ReportType_REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS:
		payload, noActivity = s.buildSignificantEventsPayload(req.Interval, req.OperatorId)
	case rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY:
		payload, noActivity = s.buildCashlessLiabilityPayload(req.Interval, req.OperatorId)
	case rgsv1.ReportType_REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT:
		payload, noActivity = s.buildAccountTransactionStatementPayload(req.Interval, req.OperatorId)
	default:
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "unsupported report_type")}, nil
	}

	var content []byte
	var contentType string
	var err error
	if req.Format == rgsv1.ReportFormat_REPORT_FORMAT_JSON {
		content, err = json.Marshal(payload)
		contentType = "application/json"
	} else {
		content, err = payloadToCSV(req.ReportType, payload)
		contentType = "text/csv"
	}
	if err != nil {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "failed to serialize report")}, nil
	}

	s.mu.Lock()
	runID := s.nextRunIDLocked()
	run := &rgsv1.ReportRun{
		ReportRunId: runID,
		ReportType:  req.ReportType,
		Interval:    req.Interval,
		Format:      req.Format,
		Status:      rgsv1.ReportRunStatus_REPORT_RUN_STATUS_COMPLETED,
		OperatorId:  req.OperatorId,
		ReportTitle: reportTitle(req.ReportType),
		GeneratedAt: s.now().Format(time.RFC3339Nano),
		NoActivity:  noActivity,
		ContentType: contentType,
		Content:     content,
	}
	s.runs[runID] = run
	s.runOrder = append(s.runOrder, runID)
	s.mu.Unlock()

	after, _ := json.Marshal(run)
	if err := s.appendAudit(req.Meta, runID, "generate_report", []byte(`{}`), after, audit.ResultSuccess, ""); err != nil {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "audit unavailable")}, nil
	}
	if err := s.persistReportRun(ctx, req.Meta, run); err != nil {
		return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
	}

	s.mu.Lock()
	s.runs[runID] = run
	s.runOrder = append(s.runOrder, runID)
	s.mu.Unlock()

	return &rgsv1.GenerateReportResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), ReportRun: cloneRun(run)}, nil
}

func (s *ReportingService) ListReportRuns(ctx context.Context, req *rgsv1.ListReportRunsRequest) (*rgsv1.ListReportRunsResponse, error) {
	if req == nil {
		req = &rgsv1.ListReportRunsRequest{}
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, "", "list_report_runs", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.ListReportRunsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	start := 0
	if req.PageToken != "" {
		if parsed, err := strconv.Atoi(req.PageToken); err == nil && parsed >= 0 {
			start = parsed
		}
	}
	size := int(req.PageSize)
	if size <= 0 {
		size = 50
	}
	if s.db != nil {
		items, err := s.listReportRunsFromDB(ctx, req.ReportTypeFilter, size, start)
		if err != nil {
			return &rgsv1.ListReportRunsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		next := ""
		if len(items) == size {
			next = strconv.Itoa(start + len(items))
		}
		return &rgsv1.ListReportRunsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), ReportRuns: items, NextPageToken: next}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	items := make([]*rgsv1.ReportRun, 0, len(s.runOrder))
	for i := len(s.runOrder) - 1; i >= 0; i-- {
		r := s.runs[s.runOrder[i]]
		if r == nil {
			continue
		}
		if req.ReportTypeFilter != rgsv1.ReportType_REPORT_TYPE_UNSPECIFIED && r.ReportType != req.ReportTypeFilter {
			continue
		}
		items = append(items, cloneRun(r))
	}

	if start > len(items) {
		start = len(items)
	}
	end := start + size
	if end > len(items) {
		end = len(items)
	}
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}

	return &rgsv1.ListReportRunsResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), ReportRuns: items[start:end], NextPageToken: next}, nil
}

func (s *ReportingService) GetReportRun(ctx context.Context, req *rgsv1.GetReportRunRequest) (*rgsv1.GetReportRunResponse, error) {
	if req == nil || req.ReportRunId == "" {
		return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(nil, rgsv1.ResultCode_RESULT_CODE_INVALID, "report_run_id is required")}, nil
	}
	if ok, reason := s.authorize(ctx, req.Meta); !ok {
		_ = s.appendAudit(req.Meta, req.ReportRunId, "get_report_run", []byte(`{}`), []byte(`{}`), audit.ResultDenied, reason)
		return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_DENIED, reason)}, nil
	}

	if s.db != nil {
		run, err := s.getReportRunFromDB(ctx, req.ReportRunId)
		if err != nil {
			return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_ERROR, "persistence unavailable")}, nil
		}
		if run == nil {
			return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "report run not found")}, nil
		}
		return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), ReportRun: run}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	run := s.runs[req.ReportRunId]
	if run == nil {
		return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_INVALID, "report run not found")}, nil
	}
	return &rgsv1.GetReportRunResponse{Meta: s.responseMeta(req.Meta, rgsv1.ResultCode_RESULT_CODE_OK, ""), ReportRun: cloneRun(run)}, nil
}
