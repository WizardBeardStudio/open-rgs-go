package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	rgsv1 "github.com/wizardbeardstudio/open-rgs-go/gen/rgs/v1"
	platformauth "github.com/wizardbeardstudio/open-rgs-go/internal/platform/auth"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestReportingGatewayParity_GenerateListGet(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 16, 0, 0, 0, time.UTC)}
	reportingSvc := NewReportingService(clk, NewLedgerService(clk), NewEventsService(clk))
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterReportingServiceHandlerServer(context.Background(), gwMux, reportingSvc); err != nil {
		t.Fatalf("register reporting gateway handlers: %v", err)
	}

	genReq := &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	}
	body, err := protojson.Marshal(genReq)
	if err != nil {
		t.Fatalf("marshal generate req: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/reporting/runs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("generate report http status: got=%d want=%d body=%s", rec.Result().StatusCode, http.StatusOK, rec.Body.String())
	}
	var genResp rgsv1.GenerateReportResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &genResp); err != nil {
		t.Fatalf("unmarshal generate response: %v", err)
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "op-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	listReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs?"+q.Encode(), nil)
	listRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listRec, listReq)
	if listRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("list report runs http status: got=%d want=%d body=%s", listRec.Result().StatusCode, http.StatusOK, listRec.Body.String())
	}
	var listResp rgsv1.ListReportRunsResponse
	if err := protojson.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list response: %v", err)
	}
	if len(listResp.ReportRuns) == 0 {
		t.Fatalf("expected at least one report run in list")
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs/"+genResp.ReportRun.ReportRunId+"?"+q.Encode(), nil)
	getRec := httptest.NewRecorder()
	gwMux.ServeHTTP(getRec, getReq)
	if getRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("get report run http status: got=%d want=%d body=%s", getRec.Result().StatusCode, http.StatusOK, getRec.Body.String())
	}
	var getResp rgsv1.GetReportRunResponse
	if err := protojson.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get response: %v", err)
	}
	if getResp.ReportRun.ReportRunId != genResp.ReportRun.ReportRunId {
		t.Fatalf("gateway get mismatch: generated=%s got=%s", genResp.ReportRun.ReportRunId, getResp.ReportRun.ReportRunId)
	}
}

func TestReportingGatewayActorMismatchDenied(t *testing.T) {
	clk := ledgerFixedClock{now: time.Date(2026, 2, 12, 16, 50, 0, 0, time.UTC)}
	reportingSvc := NewReportingService(clk, NewLedgerService(clk), NewEventsService(clk))
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterReportingServiceHandlerServer(context.Background(), gwMux, reportingSvc); err != nil {
		t.Fatalf("register reporting gateway handlers: %v", err)
	}

	genReq := &rgsv1.GenerateReportRequest{
		Meta:       meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ReportType: rgsv1.ReportType_REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY,
		Interval:   rgsv1.ReportInterval_REPORT_INTERVAL_DTD,
		Format:     rgsv1.ReportFormat_REPORT_FORMAT_JSON,
		OperatorId: "casino-1",
	}
	body, _ := protojson.Marshal(genReq)
	req := httptest.NewRequest(http.MethodPost, "/v1/reporting/runs", bytes.NewReader(body))
	req = req.WithContext(platformauth.WithActor(req.Context(), platformauth.Actor{
		ID:   "ctx-op",
		Type: "ACTOR_TYPE_OPERATOR",
	}))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("generate mismatch status: got=%d body=%s", rec.Result().StatusCode, rec.Body.String())
	}
	var genResp rgsv1.GenerateReportResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &genResp); err != nil {
		t.Fatalf("unmarshal generate mismatch response: %v", err)
	}
	if genResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied generate mismatch, got=%v", genResp.GetMeta().GetResultCode())
	}
	if genResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on generate, got=%q", genResp.GetMeta().GetDenialReason())
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

	q := make(url.Values)
	q.Set("meta.actor.actorId", "op-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	listReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs?"+q.Encode(), nil)
	listReq = listReq.WithContext(platformauth.WithActor(listReq.Context(), platformauth.Actor{
		ID:   "ctx-op",
		Type: "ACTOR_TYPE_OPERATOR",
	}))
	listRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listRec, listReq)
	if listRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("list mismatch status: got=%d body=%s", listRec.Result().StatusCode, listRec.Body.String())
	}
	var listResp rgsv1.ListReportRunsResponse
	if err := protojson.Unmarshal(listRec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list mismatch response: %v", err)
	}
	if listResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied list mismatch, got=%v", listResp.GetMeta().GetResultCode())
	}
	if listResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on list, got=%q", listResp.GetMeta().GetDenialReason())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs/"+seed.ReportRun.ReportRunId+"?"+q.Encode(), nil)
	getReq = getReq.WithContext(platformauth.WithActor(getReq.Context(), platformauth.Actor{
		ID:   "ctx-op",
		Type: "ACTOR_TYPE_OPERATOR",
	}))
	getRec := httptest.NewRecorder()
	gwMux.ServeHTTP(getRec, getReq)
	if getRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("get mismatch status: got=%d body=%s", getRec.Result().StatusCode, getRec.Body.String())
	}
	var getResp rgsv1.GetReportRunResponse
	if err := protojson.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("unmarshal get mismatch response: %v", err)
	}
	if getResp.GetMeta().GetResultCode() != rgsv1.ResultCode_RESULT_CODE_DENIED {
		t.Fatalf("expected denied get mismatch, got=%v", getResp.GetMeta().GetResultCode())
	}
	if getResp.GetMeta().GetDenialReason() != "actor mismatch with token" {
		t.Fatalf("expected actor mismatch reason on get, got=%q", getResp.GetMeta().GetDenialReason())
	}
}
