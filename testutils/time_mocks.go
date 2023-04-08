package testutils

import (
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	TIMER_MOCK_STATE_NOT_RUNNING = iota
	TIMER_MOCK_STATE_RUNNING     = iota
)

// A mockable timer provides an interface for testing, whereby the timer can be
// triggered by invoking a method rather than by having to wait. Since the
// expiration event is delivered via a time.Timer member, C, the struct is not
// directly mockable, it should be wrapped in an interface with an access method
// to the said channel.
type MockableTimer interface {
	GetChannel() <-chan time.Time
	Reset(time.Duration) bool
	Stop() bool
}

// The test driver and the unit under will run in different goroutines, so the
// former should be blocked from firing the timer before the latter
// created/reset it. Use a conditional variable to that end.
type MockTimer struct {
	cond  *sync.Cond
	C     chan time.Time
	state int
}

func (m *MockTimer) GetChannel() <-chan time.Time {
	return m.C
}

func (m *MockTimer) Reset(d time.Duration) bool {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	wasRunning := m.state == TIMER_MOCK_STATE_RUNNING
	m.state = TIMER_MOCK_STATE_RUNNING
	m.cond.Broadcast()
	return wasRunning
}

func (m *MockTimer) Stop() bool {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	wasRunning := m.state == TIMER_MOCK_STATE_RUNNING
	m.state = TIMER_MOCK_STATE_NOT_RUNNING
	return wasRunning
}

func (m *MockTimer) Fire() {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	for m.state == TIMER_MOCK_STATE_NOT_RUNNING {
		m.cond.Wait()
	}
	m.C <- time.Time{}
	m.state = TIMER_MOCK_STATE_NOT_RUNNING
}

func newMockTimer(state int) *MockTimer {
	return &MockTimer{
		cond:  sync.NewCond(&sync.Mutex{}),
		C:     make(chan time.Time, 1),
		state: state,
	}
}

func NewMockTimer() *MockTimer {
	return newMockTimer(TIMER_MOCK_STATE_RUNNING)
}

// Individual mock timers are grouped in a pool. The mock interface to NewTimer
// has an extra argument, id string, which uniquely identifies a timer. The test
// drive should have prior knowledge of how these id's are created be the unit
// under test. For instance, the timer used when waiting for an import URL to
// become healthy should use that URL as an id.
type MockTimerPool struct {
	m      *sync.Mutex
	timers map[string]*MockTimer
}

func NewMockTimePool() *MockTimerPool {
	return &MockTimerPool{
		m:      &sync.Mutex{},
		timers: make(map[string]*MockTimer),
	}
}

func (p *MockTimerPool) NewMockTimer(d time.Duration, id string) *MockTimer {
	p.m.Lock()
	defer p.m.Unlock()
	timer := p.timers[id]
	if timer == nil {
		timer = newMockTimer(TIMER_MOCK_STATE_RUNNING)
		p.timers[id] = timer
	} else {
		timer.Reset(d)
	}
	return timer
}

func (p *MockTimerPool) Fire(id string, maxWait time.Duration) (err error) {
	p.m.Lock()
	timer := p.timers[id]
	if timer == nil {
		timer = newMockTimer(TIMER_MOCK_STATE_NOT_RUNNING)
		p.timers[id] = timer
	}
	p.m.Unlock()
	if maxWait > 0 {
		ctx, cancelFn := context.WithTimeout(context.Background(), maxWait)
		fired := make(chan bool, 1)
		go func() {
			timer.Fire()
			fired <- true
		}()
		select {
		case <-ctx.Done():
			timer.Stop()
			err = fmt.Errorf("timer %s couldn't be fired after %s", id, maxWait)
		case <-fired:
		}
		cancelFn()
	} else {
		timer.Fire()
	}
	return
}
