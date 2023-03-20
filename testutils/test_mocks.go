package testutils

import (
	"time"
)

type TimeMock struct {
	now time.Time

	savedTimeNow      func() time.Time
	savedTimeNowPtr   *func() time.Time
	savedTimeSince    func(time.Time) time.Duration
	savedTimeSincePtr *func(time.Time) time.Duration
}

func (tm *TimeMock) Set(t time.Time) {
	tm.now = t
}

func (tm *TimeMock) Add(d time.Duration) {
	tm.now = tm.now.Add(d)
}

func (tm *TimeMock) Now() time.Time {
	return tm.now
}

func (tm *TimeMock) Since(t time.Time) time.Duration {
	return tm.now.Sub(t)
}

func NewTimeMock(nowFnPtr *func() time.Time, sinceFnPtr *func(time.Time) time.Duration) *TimeMock {
	tm := &TimeMock{
		now:               time.Now(),
		savedTimeNowPtr:   nowFnPtr,
		savedTimeSincePtr: sinceFnPtr,
	}
	if nowFnPtr != nil {
		tm.savedTimeNow = *nowFnPtr
		*nowFnPtr = tm.Now
	}
	if sinceFnPtr != nil {
		tm.savedTimeSince = *sinceFnPtr
		*sinceFnPtr = tm.Since
	}
	return tm
}

func (tm *TimeMock) Unmock() {
	if tm.savedTimeNowPtr != nil {
		*(tm.savedTimeNowPtr) = tm.savedTimeNow
	}
	if tm.savedTimeSincePtr != nil {
		*(tm.savedTimeSincePtr) = tm.savedTimeSince
	}
}

type HostnameClktckSecMock struct {
	savedHostname     string
	savedHostnamePtr  *string
	savedClktckSec    float64
	savedClktckSecPtr *float64
}

func (hcm *HostnameClktckSecMock) Unmock() {
	if hcm.savedHostnamePtr != nil {
		*(hcm.savedHostnamePtr) = hcm.savedHostname
	}
	if hcm.savedClktckSecPtr != nil {
		*(hcm.savedClktckSecPtr) = hcm.savedClktckSec
	}
}

func (hcm *HostnameClktckSecMock) Set(h string, c float64) {
	if hcm.savedHostnamePtr != nil {
		*(hcm.savedHostnamePtr) = h
	}
	if hcm.savedClktckSecPtr != nil {
		*(hcm.savedClktckSecPtr) = c
	}
}

func NewHostnameClktckSecMock(hPtr *string, cPtr *float64) *HostnameClktckSecMock {
	hcm := &HostnameClktckSecMock{
		savedHostnamePtr:  hPtr,
		savedClktckSecPtr: cPtr,
	}
	if hPtr != nil {
		hcm.savedHostname = *hPtr
	}
	if cPtr != nil {
		hcm.savedClktckSec = *cPtr
	}
	return hcm
}
