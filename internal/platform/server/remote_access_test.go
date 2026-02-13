package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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

func TestRemoteAccessGuardDisableInMemoryActivityCache(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	guard.SetDisableInMemoryActivityCache(true)

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
	if len(logs) != 0 {
		t.Fatalf("expected no in-memory activities when cache disabled, got=%d", len(logs))
	}
}

func TestRemoteAccessGuardFailClosedOnLogPersistenceFailure(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	db, err := sql.Open("pgx", "postgres://127.0.0.1:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("open db err: %v", err)
	}
	_ = db.Close()
	guard.SetDB(db)
	guard.SetFailClosedOnLogPersistenceFailure(true)

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	req.RemoteAddr = "127.0.0.1:44000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected service unavailable when log persistence fails closed, got=%d", rec.Result().StatusCode)
	}
}

func TestRemoteAccessGuardAllowsWhenLogPersistenceFailureNotFailClosed(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	db, err := sql.Open("pgx", "postgres://127.0.0.1:5432/postgres?sslmode=disable")
	if err != nil {
		t.Fatalf("open db err: %v", err)
	}
	_ = db.Close()
	guard.SetDB(db)
	guard.SetFailClosedOnLogPersistenceFailure(false)

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	req.RemoteAddr = "127.0.0.1:44000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected ok when fail-closed logging is disabled, got=%d", rec.Result().StatusCode)
	}
}

func TestRemoteAccessGuardFailClosedWhenInMemoryActivityCapExceeded(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	guard.SetFailClosedOnLogPersistenceFailure(true)
	guard.SetInMemoryActivityLogCap(1)

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	firstReq.RemoteAddr = "127.0.0.1:44000"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, firstReq)
	if firstRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first request to pass, got=%d", firstRec.Result().StatusCode)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	secondReq.RemoteAddr = "127.0.0.1:44000"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, secondReq)
	if secondRec.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected service unavailable after activity-cap reached, got=%d", secondRec.Result().StatusCode)
	}
}

func TestRemoteAccessGuardCapExceededDoesNotFailClosedWhenDisabled(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	guard.SetFailClosedOnLogPersistenceFailure(false)
	guard.SetInMemoryActivityLogCap(1)

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	firstReq.RemoteAddr = "127.0.0.1:44000"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, firstReq)
	if firstRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first request to pass, got=%d", firstRec.Result().StatusCode)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	secondReq.RemoteAddr = "127.0.0.1:44000"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, secondReq)
	if secondRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected second request to pass when fail-closed is disabled, got=%d", secondRec.Result().StatusCode)
	}
}

func TestRemoteAccessGuardDecisionObserver(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	var outcomes []string
	guard.SetDecisionObserver(func(outcome string) {
		outcomes = append(outcomes, outcome)
	})

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	allowedReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	allowedReq.RemoteAddr = "127.0.0.1:44000"
	allowedRec := httptest.NewRecorder()
	h.ServeHTTP(allowedRec, allowedReq)
	if allowedRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected allowed request to pass, got=%d", allowedRec.Result().StatusCode)
	}

	deniedReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	deniedReq.RemoteAddr = "203.0.113.8:45000"
	deniedRec := httptest.NewRecorder()
	h.ServeHTTP(deniedRec, deniedReq)
	if deniedRec.Result().StatusCode != http.StatusForbidden {
		t.Fatalf("expected denied request to be forbidden, got=%d", deniedRec.Result().StatusCode)
	}

	if len(outcomes) != 2 || outcomes[0] != "allowed" || outcomes[1] != "denied" {
		t.Fatalf("unexpected observer outcomes: %v", outcomes)
	}
}

func TestRemoteAccessGuardLogStateObserver(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	type state struct {
		entries int
		cap     int
	}
	var states []state
	guard.SetLogStateObserver(func(entries int, cap int) {
		states = append(states, state{entries: entries, cap: cap})
	})
	guard.SetInMemoryActivityLogCap(2)

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	req.RemoteAddr = "127.0.0.1:44000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected request to pass, got=%d", rec.Result().StatusCode)
	}

	if len(states) < 3 {
		t.Fatalf("expected at least 3 state observations, got=%d", len(states))
	}
	last := states[len(states)-1]
	if last.entries != 1 || last.cap != 2 {
		t.Fatalf("unexpected last log state: %+v", last)
	}
}

func TestRemoteAccessGuardObserverEmitsLoggingUnavailableInFailOpenMode(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	guard.SetFailClosedOnLogPersistenceFailure(false)
	guard.SetInMemoryActivityLogCap(1)
	var outcomes []string
	guard.SetDecisionObserver(func(outcome string) {
		outcomes = append(outcomes, outcome)
	})

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	firstReq.RemoteAddr = "127.0.0.1:44000"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, firstReq)
	if firstRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first request to pass, got=%d", firstRec.Result().StatusCode)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	secondReq.RemoteAddr = "127.0.0.1:44000"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, secondReq)
	if secondRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected second request to pass in fail-open mode, got=%d", secondRec.Result().StatusCode)
	}

	expected := []string{"allowed", "logging_unavailable", "allowed"}
	if len(outcomes) != len(expected) {
		t.Fatalf("unexpected outcomes length: got=%v want=%v", outcomes, expected)
	}
	for i := range expected {
		if outcomes[i] != expected[i] {
			t.Fatalf("unexpected outcomes order: got=%v want=%v", outcomes, expected)
		}
	}
}

func TestRemoteAccessGuardObserverEmitsLoggingUnavailableInFailClosedMode(t *testing.T) {
	guard, err := NewRemoteAccessGuard(ledgerFixedClock{now: time.Date(2026, 2, 12, 18, 0, 0, 0, time.UTC)}, nil, []string{"127.0.0.1/32"})
	if err != nil {
		t.Fatalf("new guard err: %v", err)
	}
	guard.SetFailClosedOnLogPersistenceFailure(true)
	guard.SetInMemoryActivityLogCap(1)
	var outcomes []string
	guard.SetDecisionObserver(func(outcome string) {
		outcomes = append(outcomes, outcome)
	})

	h := guard.Wrap(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	firstReq.RemoteAddr = "127.0.0.1:44000"
	firstRec := httptest.NewRecorder()
	h.ServeHTTP(firstRec, firstReq)
	if firstRec.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected first request to pass, got=%d", firstRec.Result().StatusCode)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/reporting/runs", nil)
	secondReq.RemoteAddr = "127.0.0.1:44000"
	secondRec := httptest.NewRecorder()
	h.ServeHTTP(secondRec, secondReq)
	if secondRec.Result().StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected second request to fail closed, got=%d", secondRec.Result().StatusCode)
	}

	expected := []string{"allowed", "logging_unavailable"}
	if len(outcomes) != len(expected) {
		t.Fatalf("unexpected outcomes length: got=%v want=%v", outcomes, expected)
	}
	for i := range expected {
		if outcomes[i] != expected[i] {
			t.Fatalf("unexpected outcomes order: got=%v want=%v", outcomes, expected)
		}
	}
}
