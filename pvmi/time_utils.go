// time package function re-directors useful for test mocking

package pvmi

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

type MockTimer struct {
	C       chan time.Time
	running bool
}

func (m *MockTimer) GetChannel() <-chan time.Time {
	return m.C
}

func (m *MockTimer) Reset(d time.Duration) {
	m.running = true
}

func (m *MockTimer) Stop() bool {
	wasRunning := m.running
	m.running = false
	return wasRunning
}

func (m *MockTimer) Fire() {
	if m.running {
		m.running = false
		m.C <- time.Time{}
	}
}

func NewMockTimer(d time.Duration, id string) *MockTimer {
	return &MockTimer{
		C:       make(chan time.Time, 1),
		running: true,
	}
}
