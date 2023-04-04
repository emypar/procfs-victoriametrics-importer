// time package function re-directors useful for test mocking

package utils

import "time"

const (
	ISO8601Micro = "2006-01-02T15:04:05.999999-07:00"
)

var TimeNow func() time.Time = time.Now
var TimeSince func(time.Time) time.Duration = time.Since

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
