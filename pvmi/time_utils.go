// time package function re-directors useful for test mocking

package pvmi

import (
	"context"
	"sync"
	"time"
)

var TimeUtilsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"TimeUtils",
)

// Mock time.Now():
type TimeNowFn func() time.Time

// Define cancellable pause objects, useful for testing. They are similar to a
// cancellable context w/ deadline, with 2 distinctions:
// * they consist of reusable objects, being more efficient for long lived pause
//   in a loop
// * they have an ID so they may come from a pool; this way they can be
//   created in a goroutine in the unit under test and they can be controlled
//   from the test goroutine (as long as the id is known)

type CancelablePauseTimer interface {
	PauseUntil(deadline time.Time) (cancelled bool)
	Cancel()
	CancelWait()
}

type CancelableTimer struct {
	timer     *time.Timer
	ctx       context.Context
	cancelFn  context.CancelFunc
	cond      *sync.Cond
	pausing   bool
	cancelled bool
}

type NewCancelableTimerFn func(parentCtx context.Context, id string) CancelablePauseTimer

func NewCancelableTimer(
	parentCtx context.Context,
	// needed for testing to a identify a mock cancellable timer in a pool:
	id string,
) CancelablePauseTimer {
	timer := time.NewTimer(time.Duration(0))
	<-timer.C
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancelFn := context.WithCancel(parentCtx)
	return CancelablePauseTimer(&CancelableTimer{
		timer:     timer,
		ctx:       ctx,
		cancelFn:  cancelFn,
		cond:      sync.NewCond(&sync.Mutex{}),
		pausing:   false,
		cancelled: false,
	})
}

func (t *CancelableTimer) PauseUntil(deadline time.Time) (cancelled bool) {
	defer func() {
		t.cond.L.Lock()
		// If it transitioned to cancelled, notify potentially blocked
		// CancelWait() to unblock:
		if !t.cancelled && cancelled {
			t.cond.Broadcast()
		}
		t.cancelled, t.pausing = cancelled, false
		t.cond.L.Unlock()
	}()

	// If already cancelled, return right away:
	t.cond.L.Lock()
	cancelled, t.pausing = t.cancelled, true
	t.cond.L.Unlock()
	if cancelled {
		return
	}
	pause := time.Until(deadline)
	if pause <= 0 {
		// No timer required, simply check for cancel and return right away:
		select {
		case <-t.ctx.Done():
			cancelled = true
		default:
		}
		return
	}

	// It has been found on ubuntu docker that the timer may fire prematurely in
	// a goroutine (to be investigated further). In the mean time, to make this
	// reliable, loop in an adjusted pause until it drops to <= 0:
	firstPass, initialPause := true, pause
	for pause > 0 {
		if !firstPass {
			TimeUtilsLog.Warnf(
				"Timer fired prematurely: initialPause=%s, remaining pause=%s",
				initialPause,
				pause,
			)
			pause = time.Until(deadline) // to account for the logger time
		}
		t.timer.Reset(pause)
		select {
		case <-t.ctx.Done():
			if !t.timer.Stop() {
				<-t.timer.C
			}
			cancelled = true
			return
		case <-t.timer.C:
		}
		pause = time.Until(deadline)
		firstPass = false
	}
	return
}

func (t *CancelableTimer) Cancel() {
	t.cond.L.Lock()
	defer t.cond.L.Unlock()

	if !t.cancelled {
		t.cancelFn()
	}
}

func (t *CancelableTimer) CancelWait() {
	t.cond.L.Lock()
	defer t.cond.L.Unlock()
	if t.cancelled {
		return
	}
	t.cancelFn()
	for t.pausing && !t.cancelled {
		t.cond.Wait()
	}
}
