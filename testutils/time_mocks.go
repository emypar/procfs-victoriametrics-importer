package testutils

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// A cancelable timer mock should allow it to be fired before pause is invoked,
// with fire action taking effect once the latter occurs. This is because Pause
// and Fire are invoked from different, unsynchronized goroutines: Pause from
// the system under test and Fire from the test framework.
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
		// If it transitions to cancelled, notify potentially blocked
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

// Individual mock timers are grouped in a pool. The mock interface to
// NewCancelableTimer has an extra argument, id string, which uniquely
// identifies a timer. The test drive should have prior knowledge of how these
// id's are created be the unit under test. For instance, the timer used when
// waiting for an import URL to become healthy should use that URL as an id.
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
