// time package function re-directors useful for test mocking

package pvmi

import "time"

// A mockable timer provides an interface for testing, whereby the timer can be
// triggered by invoking a method rather than by having to wait.
type MockableTimer interface {
	GetChannel() <-chan time.Time
	Reset(time.Duration) bool
	Stop() bool
}

type RealTimer struct {
	*time.Timer
}

func (t *RealTimer) GetChannel() <-chan time.Time {
	return t.C
}

type NewMockableTimerFn func(time.Duration, string) MockableTimer

func NewRealTimer(d time.Duration, id string) MockableTimer {
	return MockableTimer(&RealTimer{time.NewTimer(d)})
}
