package audit

import (
	"testing"
	"time"
)

func TestAppendChainsEvents(t *testing.T) {
	s := NewInMemoryStore()
	now := time.Date(2026, 2, 11, 0, 0, 0, 0, time.UTC)

	first, err := s.Append(Event{
		AuditID:    "a1",
		RecordedAt: now,
		ActorID:    "operator-1",
		Action:     "login",
		Result:     ResultSuccess,
	})
	if err != nil {
		t.Fatalf("append first: %v", err)
	}
	if first.HashPrev != "GENESIS" || first.HashCurr == "" {
		t.Fatalf("unexpected hash chain on first event: %+v", first)
	}

	second, err := s.Append(Event{
		AuditID:    "a2",
		RecordedAt: now.Add(time.Second),
		ActorID:    "operator-1",
		Action:     "logout",
		Result:     ResultSuccess,
	})
	if err != nil {
		t.Fatalf("append second: %v", err)
	}
	if second.HashPrev != first.HashCurr {
		t.Fatalf("expected chain link, got prev=%s want=%s", second.HashPrev, first.HashCurr)
	}
}
