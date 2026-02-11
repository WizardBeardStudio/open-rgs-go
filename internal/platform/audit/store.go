package audit

import (
	"errors"
	"sync"
)

var ErrCorruptChain = errors.New("audit chain corruption detected")

type InMemoryStore struct {
	mu     sync.Mutex
	events []Event
	last   string
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{last: "GENESIS"}
}

func (s *InMemoryStore) Append(e Event) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	e.HashPrev = s.last
	e.HashCurr = ComputeHash(s.last, e)

	if len(s.events) > 0 {
		prev := s.events[len(s.events)-1]
		recomputed := ComputeHash(prev.HashPrev, prev)
		if recomputed != prev.HashCurr {
			return Event{}, ErrCorruptChain
		}
	}

	s.events = append(s.events, e)
	s.last = e.HashCurr
	return e, nil
}

func (s *InMemoryStore) Events() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Event, len(s.events))
	copy(out, s.events)
	return out
}
