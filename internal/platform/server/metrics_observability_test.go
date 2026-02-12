package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
