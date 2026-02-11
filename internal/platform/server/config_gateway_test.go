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

func TestConfigGatewayParity_Workflow(t *testing.T) {
	svc := NewConfigService(ledgerFixedClock{now: time.Date(2026, 2, 12, 17, 30, 0, 0, time.UTC)})
	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterConfigServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register config gateway handlers: %v", err)
	}

	proposeReq := &rgsv1.ProposeConfigChangeRequest{
		Meta:            meta("op-1", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ConfigNamespace: "security",
		ConfigKey:       "session_timeout",
		ProposedValue:   "1200",
		Reason:          "gateway test",
	}
	body, _ := protojson.Marshal(proposeReq)
	req := httptest.NewRequest(http.MethodPost, "/v1/config/changes:propose", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("propose http status: got=%d want=%d body=%s", rec.Result().StatusCode, http.StatusOK, rec.Body.String())
	}
	var proposeResp rgsv1.ProposeConfigChangeResponse
	if err := protojson.Unmarshal(rec.Body.Bytes(), &proposeResp); err != nil {
		t.Fatalf("unmarshal propose response: %v", err)
	}

	approveReq := &rgsv1.ApproveConfigChangeRequest{
		Meta:     meta("op-2", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposeResp.Change.ChangeId,
		Reason:   "gateway approve",
	}
	approveBody, _ := protojson.Marshal(approveReq)
	approveHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/config/changes/"+proposeResp.Change.ChangeId+":approve", bytes.NewReader(approveBody))
	approveHTTPReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	gwMux.ServeHTTP(approveRec, approveHTTPReq)
	if approveRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("approve http status: got=%d want=%d body=%s", approveRec.Result().StatusCode, http.StatusOK, approveRec.Body.String())
	}

	applyReq := &rgsv1.ApplyConfigChangeRequest{
		Meta:     meta("op-3", rgsv1.ActorType_ACTOR_TYPE_OPERATOR, ""),
		ChangeId: proposeResp.Change.ChangeId,
		Reason:   "gateway apply",
	}
	applyBody, _ := protojson.Marshal(applyReq)
	applyHTTPReq := httptest.NewRequest(http.MethodPost, "/v1/config/changes/"+proposeResp.Change.ChangeId+":apply", bytes.NewReader(applyBody))
	applyHTTPReq.Header.Set("Content-Type", "application/json")
	applyRec := httptest.NewRecorder()
	gwMux.ServeHTTP(applyRec, applyHTTPReq)
	if applyRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("apply http status: got=%d want=%d body=%s", applyRec.Result().StatusCode, http.StatusOK, applyRec.Body.String())
	}

	q := make(url.Values)
	q.Set("meta.actor.actorId", "op-1")
	q.Set("meta.actor.actorType", "ACTOR_TYPE_OPERATOR")
	listReq := httptest.NewRequest(http.MethodGet, "/v1/config/history?"+q.Encode(), nil)
	listRec := httptest.NewRecorder()
	gwMux.ServeHTTP(listRec, listReq)
	if listRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("list history http status: got=%d want=%d body=%s", listRec.Result().StatusCode, http.StatusOK, listRec.Body.String())
	}
	var history rgsv1.ListConfigHistoryResponse
	if err := protojson.Unmarshal(listRec.Body.Bytes(), &history); err != nil {
		t.Fatalf("unmarshal history response: %v", err)
	}
	if len(history.Changes) == 0 {
		t.Fatalf("expected at least one config change in history")
	}
}
