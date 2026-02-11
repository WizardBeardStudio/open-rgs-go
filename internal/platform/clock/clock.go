package clock

import "time"

// Clock allows deterministic time behavior in tests and replay flows.
type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time {
	return time.Now().UTC()
}
