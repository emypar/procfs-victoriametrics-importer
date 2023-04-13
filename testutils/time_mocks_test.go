// my test gear has tests

package testutils

import (
	"context"
	"testing"
	"time"
)

const (
	TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE = 100 * time.Millisecond
)

func TestCancelableTimerMockCancelBeforeFire(t *testing.T) {
	parentCtx, _ := context.WithCancel(context.Background())
	cancelableTimerMock := NewCancelableTimerMock(parentCtx)
	pauseRetValC := make(chan bool, 1)
	go func() {
		pauseRetValC <- cancelableTimerMock.PauseUntil(time.Unix(0, 0))
	}()
	time.Sleep(TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	cancelableTimerMock.Cancel()
	ctx, cancelFn := context.WithTimeout(context.Background(), TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	select {
	case retVal := <-pauseRetValC:
		cancelFn()
		if !retVal {
			t.Fatalf("PauseUntil()=%v", retVal)
		}
	case <-ctx.Done():
		cancelFn()
		t.Fatal("PauseUntil() didn't return")
	}
}

func TestCancelableTimerMockCancelParentBeforeFire(t *testing.T) {
	parentCtx, parentCancelFn := context.WithCancel(context.Background())
	cancelableTimerMock := NewCancelableTimerMock(parentCtx)
	pauseRetValC := make(chan bool, 1)
	go func() {
		pauseRetValC <- cancelableTimerMock.PauseUntil(time.Unix(0, 0))
	}()
	time.Sleep(TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	parentCancelFn()
	ctx, cancelFn := context.WithTimeout(context.Background(), TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	select {
	case retVal := <-pauseRetValC:
		cancelFn()
		if !retVal {
			t.Fatalf("PauseUntil()=%v", retVal)
		}
	case <-ctx.Done():
		cancelFn()
		t.Fatal("PauseUntil() didn't return")
	}
}

func TestCancelableTimerMockFireBefore(t *testing.T) {
	parentCtx, _ := context.WithCancel(context.Background())
	cancelableTimerMock := NewCancelableTimerMock(parentCtx)
	pauseRetValC := make(chan bool, 1)
	go func() {
		time.Sleep(TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
		pauseRetValC <- cancelableTimerMock.PauseUntil(time.Unix(0, 0))
	}()
	err := cancelableTimerMock.FireWithTimeout(2 * TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	select {
	case retVal := <-pauseRetValC:
		cancelFn()
		if retVal {
			t.Fatalf("PauseUntil()=%v", retVal)
		}
	case <-ctx.Done():
		cancelFn()
		t.Fatal("PauseUntil() didn't return")
	}
}

func TestCancelableTimerMockFireBeforeTimeout(t *testing.T) {
	parentCtx, _ := context.WithCancel(context.Background())
	cancelableTimerMock := NewCancelableTimerMock(parentCtx)
	pauseRetValC := make(chan bool, 1)
	go func() {
		time.Sleep(2 * TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
		pauseRetValC <- cancelableTimerMock.PauseUntil(time.Unix(0, 0))
	}()
	err := cancelableTimerMock.FireWithTimeout(TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	if err == nil {
		t.Fatal("FireWithTimeout() didn't timeout")
	} else {
		t.Log(err)
	}
	ctx, cancelFn := context.WithTimeout(context.Background(), 3*TEST_CANCELABLE_TIMER_MOCK_BASE_PAUSE)
	select {
	case retVal := <-pauseRetValC:
		cancelFn()
		if retVal {
			t.Fatalf("PauseUntil()=%v", retVal)
		}
	case <-ctx.Done():
		cancelFn()
		t.Fatal("PauseUntil() didn't return")
	}
}
