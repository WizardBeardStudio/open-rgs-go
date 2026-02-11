package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteAccessGuardDeniesUntrustedAdminPath(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/config/history", nil)
	req.RemoteAddr = "203.0.113.8:45000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected forbidden for untrusted admin path, got=%d", rec.Result().StatusCode)
	}
	logs := guard.Activities()
	if len(logs) != 1 || logs[0].Allowed {
		t.Fatalf("expected one denied activity log")
	}
}

func TestRemoteAccessGuardAllowsTrustedAdminPath(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	req.RemoteAddr = "127.0.0.1:44000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected ok for trusted admin path, got=%d", rec.Result().StatusCode)
	}
	logs := guard.Activities()
	if len(logs) != 1 || !logs[0].Allowed {
		t.Fatalf("expected one allowed activity log")
	}
}
