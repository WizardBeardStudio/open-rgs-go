package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	metricsTestOnce sync.Once
	metricsTestInst *Metrics
)

func metricsForTest() *Metrics {
	metricsTestOnce.Do(func() {
		metricsTestInst = NewMetrics()
	})
	return metricsTestInst
}

func counterValue(t *testing.T, metricName string, labels map[string]string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != metricName {
			continue
		}
		for _, m := range fam.GetMetric() {
			if metricLabelsMatch(m, labels) && m.GetCounter() != nil {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func gaugeValue(t *testing.T, metricName string) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() != metricName {
			continue
		}
		for _, m := range fam.GetMetric() {
			if m.GetGauge() != nil {
				return m.GetGauge().GetValue()
			}
		}
	}
	return 0
}

func metricLabelsMatch(metric *dto.Metric, expected map[string]string) bool {
	if len(expected) == 0 {
		return true
	}
	actual := make(map[string]string, len(metric.GetLabel()))
	for _, lp := range metric.GetLabel() {
		actual[lp.GetName()] = lp.GetValue()
	}
	for k, v := range expected {
		if actual[k] != v {
			return false
		}
	}
	return true
}

func TestGRPCCodeFromHTTPStatus(t *testing.T) {
	cases := []struct {
		statusCode int
		want       codes.Code
	}{
		{statusCode: 200, want: codes.OK},
		{statusCode: 400, want: codes.InvalidArgument},
		{statusCode: 401, want: codes.Unauthenticated},
		{statusCode: 403, want: codes.PermissionDenied},
		{statusCode: 404, want: codes.NotFound},
		{statusCode: 409, want: codes.Aborted},
		{statusCode: 429, want: codes.ResourceExhausted},
		{statusCode: 422, want: codes.FailedPrecondition},
		{statusCode: 500, want: codes.Internal},
	}
	for _, tc := range cases {
		got := grpcCodeFromHTTPStatus(tc.statusCode)
		if got != tc.want {
			t.Fatalf("status=%d got=%s want=%s", tc.statusCode, got.String(), tc.want.String())
		}
	}
}

func TestHTTPMetricsMiddlewarePreservesStatus(t *testing.T) {
	handler := HTTPMetricsMiddleware(nil, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	req := httptest.NewRequest(http.MethodGet, "/v1/config/history", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unexpected status=%d", rec.Code)
	}
}

func TestUnaryMetricsInterceptorPassesThroughError(t *testing.T) {
	interceptor := UnaryMetricsInterceptor(nil)
	handlerErr := status.Error(codes.PermissionDenied, "denied")
	_, err := interceptor(context.Background(), "req", &grpc.UnaryServerInfo{FullMethod: "/rgs.v1.LedgerService/Deposit"}, func(context.Context, interface{}) (interface{}, error) {
		return nil, handlerErr
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("expected permission denied, got=%s", status.Code(err).String())
	}
}

func TestMetricsObserveRemoteAccessDecision(t *testing.T) {
	m := metricsForTest()
	before := counterValue(t, "open_rgs_remote_access_decisions_total", map[string]string{"outcome": "allowed"})
	m.ObserveRemoteAccessDecision("allowed")
	after := counterValue(t, "open_rgs_remote_access_decisions_total", map[string]string{"outcome": "allowed"})
	if after != before+1 {
		t.Fatalf("expected allowed counter increment by 1, before=%f after=%f", before, after)
	}
}

func TestMetricsObserveRemoteAccessLogState(t *testing.T) {
	m := metricsForTest()
	m.ObserveRemoteAccessLogState(12, 50)

	entries := gaugeValue(t, "open_rgs_remote_access_inmemory_log_entries")
	capacity := gaugeValue(t, "open_rgs_remote_access_inmemory_log_cap")
	if entries != 12 {
		t.Fatalf("expected entries gauge=12, got=%f", entries)
	}
	if capacity != 50 {
		t.Fatalf("expected cap gauge=50, got=%f", capacity)
	}
}
