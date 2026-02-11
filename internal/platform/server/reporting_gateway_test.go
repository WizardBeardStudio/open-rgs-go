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
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
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
