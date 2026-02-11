package server

import (
	"context"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	rgsv1 "github.com/wizardbeard/open-rgs-go/gen/rgs/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type fixedClock struct {
	now time.Time
}

func (f fixedClock) Now() time.Time {
	return f.now
}

func TestSystemStatusGateway(t *testing.T) {
	startedAt := time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
	clk := fixedClock{now: startedAt.Add(5 * time.Minute)}
	svc := SystemService{
		StartedAt: startedAt,
		Clock:     clk,
		Version:   "test-version",
	}

	gwMux := runtime.NewServeMux()
	if err := rgsv1.RegisterSystemServiceHandlerServer(context.Background(), gwMux, svc); err != nil {
		t.Fatalf("register gateway handlers: %v", err)
	}

	req := httptest.NewRequest("GET", "/v1/system/status", nil)
	rec := httptest.NewRecorder()
	gwMux.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("unexpected status code: got=%d want=%d", resp.StatusCode, 200)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var got rgsv1.GetSystemStatusResponse
	if err := protojson.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal response: %v; body=%s", err, string(body))
	}

	if got.ServiceName != "open-rgs-go" {
		t.Fatalf("service name mismatch: got=%q", got.ServiceName)
	}
	if got.Version != "test-version" {
		t.Fatalf("version mismatch: got=%q", got.Version)
	}
	if got.Meta == nil || got.Meta.ResultCode != rgsv1.ResultCode_RESULT_CODE_OK {
		t.Fatalf("missing/invalid response meta: %+v", got.Meta)
	}
}
