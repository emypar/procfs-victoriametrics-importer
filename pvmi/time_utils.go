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
	Reset(time.Duration)
	Stop() bool
	Fire()
}

type RealTimer struct {
	timer *time.Timer
}

func (t *RealTimer) GetChannel() <-chan time.Time {
	return t.timer.C
}

func (t *RealTimer) Reset(d time.Duration) {
	t.timer.Reset(d)
}

func (t *RealTimer) Stop() bool {
	return t.timer.Stop()
}

func (t *RealTimer) Fire() {
}

func NewRealTimer(d time.Duration) *RealTimer {
	return &RealTimer{
		timer: time.NewTimer(d),
	}
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

func NewMockTimer(d time.Duration) *MockTimer {
	return &MockTimer{
		C:       make(chan time.Time, 1),
		running: true,
	}
}
