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
			err = fmt.Errorf("timer %s couldn'm be fired after %s", id, maxWait)
		case <-fired:
		}
		cancelFn()
	} else {
		timer.Fire()
	}
	return
}

type CancelableTimerMock struct {
	ctx       context.Context
	cancelFn  context.CancelFunc
	cond      *sync.Cond
	pausing   bool
	cancelled bool
	C         chan bool
}

func NewCancelableTimerMock(
	parentCtx context.Context,
) *CancelableTimerMock {
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancelFn := context.WithCancel(parentCtx)
	return &CancelableTimerMock{
		ctx:       ctx,
		cancelFn:  cancelFn,
		cond:      sync.NewCond(&sync.Mutex{}),
		pausing:   false,
		cancelled: false,
		C:         make(chan bool, 1),
	}
}

func (m *CancelableTimerMock) PauseUntil(deadline time.Time) (cancelled bool) {
	defer func() {
		m.cond.L.Lock()
		// If it transitioned to cancelled, notify potentially blocked
		// CancelWait() and Fire...() to unblock:
		if !m.cancelled && cancelled {
			m.cond.Broadcast()
		}
		m.cancelled, m.pausing = cancelled, false
		m.cond.L.Unlock()
	}()

	// If already cancelled, return right away:
	if cancelled {
		return
	}

	// Signal wait on pausing, if any:
	m.cond.L.Lock()
	m.pausing = true
	m.cond.Broadcast()
	m.cond.L.Unlock()
	select {
	case <-m.ctx.Done():
		cancelled = true
		return
	case <-m.C:
	}
	return
}

func (m *CancelableTimerMock) Fire() {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	if !m.pausing && !m.cancelled {
		m.cond.Wait()
	}
	if m.pausing {
		m.C <- true
	}
}

func (m *CancelableTimerMock) FireWithTimeout(timeout time.Duration) (err error) {
	ctx, cancelFn := context.WithTimeout(context.Background(), timeout)
	doneC := make(chan bool, 1)
	go func() {
		m.Fire()
		doneC <- true
	}()
	select {
	case <-ctx.Done():
		err = fmt.Errorf("couldn't fire after %s", timeout)
	case <-doneC:
	}
	cancelFn()
	return
}

func (m *CancelableTimerMock) Cancel() {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	if !m.cancelled {
		m.cancelFn()
	}
}

func (m *CancelableTimerMock) CancelWait() {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()
	if m.cancelled {
		return
	}
	m.cancelFn()
	for m.pausing && !m.cancelled {
		m.cond.Wait()
	}
}

type CancelableTimerMockPool struct {
	m      *sync.Mutex
	timers map[string]*CancelableTimerMock
}

func NewCancelableTimerMockPool() *CancelableTimerMockPool {
	return &CancelableTimerMockPool{
		m:      &sync.Mutex{},
		timers: make(map[string]*CancelableTimerMock),
	}
}

func (p *CancelableTimerMockPool) NewCancelableTimer(parentCtx context.Context, id string) *CancelableTimerMock {
	p.m.Lock()
	defer p.m.Unlock()
	timer := p.timers[id]
	if timer == nil {
		timer = NewCancelableTimerMock(parentCtx)
		p.timers[id] = timer
	}
	return timer
}

func (p *CancelableTimerMockPool) Fire(parentCtx context.Context, id string) {
	p.m.Lock()
	timer := p.timers[id]
	if timer == nil {
		timer = NewCancelableTimerMock(parentCtx)
		p.timers[id] = timer
	}
	p.m.Unlock()
	timer.Fire()
}

func (p *CancelableTimerMockPool) FireWithTimeout(
	parentCtx context.Context,
	id string,
	timeout time.Duration,
) error {
	p.m.Lock()
	timer := p.timers[id]
	if timer == nil {
		timer = NewCancelableTimerMock(parentCtx)
		p.timers[id] = timer
	}
	p.m.Unlock()
	return timer.FireWithTimeout(timeout)
}

type TimeNowMock struct {
	now time.Time
}

func NewTimeNowMock() *TimeNowMock {
	return &TimeNowMock{time.Unix(0, 0)}
}

func (m *TimeNowMock) Set(t time.Time) {
	m.now = t
}

func (m *TimeNowMock) Now() time.Time {
	return m.now
}
