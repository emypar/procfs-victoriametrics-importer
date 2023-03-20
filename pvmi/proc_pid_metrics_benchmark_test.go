// Benchmarks for pid cache implementation.
//
// The PID cache is a map[int]*... used to preserve state from one pass to the
// next one for pseudo-categorical metrics and %CPU.
//
// PID's may go out of scope from one pass to the next and this benchmark
// evaluates 2 solutions:
//
// Single cache: each cache entry has a pass# field, updated for all the PIDs
// found in the current pass. At the end the entire cache is scanned looking for
// entries that do not have pass# updated and those are deemed out of scope.
//
// Dual cache: ach pass creates a new cache with PID's found at the current
// pass. The PID's are removed from the previous pass cache and what is left in
// the latter are out of scope PID's.
//
// Invocation:
//     go test -bench ^BenchmarkPid.*Cache -run ^BenchmarkPid.*Cache -cpu=2

package pvmi

import (
	"fmt"
	"testing"
)

type TestPidCacheEntry struct {
	PidMetricsCacheEntry
	testPassNum int
}

type TestPidCache map[PidTidPair]*TestPidCacheEntry

type PidCacheTestCase struct {
	pidCount, changedPidCount int
}

var pidCacheTestCases = [...]PidCacheTestCase{
	{10_000, 0},
	{10_000, 100},
	{10_000, 1_000},
	{100_000, 0},
	{100_000, 100},
	{100_000, 1_000},
	{100_000, 10_000},
	{1_000_000, 0},
	{1_000_000, 100},
	{1_000_000, 1_000},
	{1_000_000, 10_000},
	{1_000_000, 100_000},
}

func benchmarkPidSingleCache(
	b *testing.B,
	pidCount, changedPidCount int,
) {
	pidCache := TestPidCache{}
	initPassNum := -1
	for pid := 1; pid <= pidCount; pid++ {
		pidTid := PidTidPair{pid: pid, tid: 0}
		pidCache[pidTid] = &TestPidCacheEntry{
			testPassNum: initPassNum,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		passNum := int(i)
		startPid, endPid := 1, pidCount
		if i&1 == 1 {
			startPid, endPid = startPid+changedPidCount, endPid+changedPidCount
		}
		for pid := startPid; pid <= endPid; pid++ {
			pidTid := PidTidPair{pid: pid, tid: 0}
			pcme := pidCache[pidTid]
			if pcme == nil {
				pcme = &TestPidCacheEntry{}
				pidCache[pidTid] = pcme
			}
			pcme.testPassNum = passNum
		}
		for pidTid, pcme := range pidCache {
			if pcme.testPassNum != passNum {
				delete(pidCache, pidTid)
			}
		}
	}
}

func benchmarkPidDualCache(
	b *testing.B,
	pidCount, changedPidCount int,
) {
	pidCache := TestPidCache{}
	initPassNum := -1
	for pid := 1; pid <= pidCount; pid++ {
		pidTid := PidTidPair{pid: pid, tid: 0}
		pidCache[pidTid] = &TestPidCacheEntry{
			testPassNum: initPassNum,
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		passNum := int(i)
		startPid, endPid := 1, pidCount
		if i&1 == 1 {
			startPid, endPid = startPid+changedPidCount, endPid+changedPidCount
		}
		prevPidCache := pidCache
		for pid := startPid; pid <= endPid; pid++ {
			pidTid := PidTidPair{pid: pid, tid: 0}
			pcme := prevPidCache[pidTid]
			if pcme == nil {
				pcme = &TestPidCacheEntry{}
				pidCache[pidTid] = pcme
			} else {
				pidCache[pidTid] = pcme
				delete(prevPidCache, pidTid)
			}
			pcme.testPassNum = passNum
		}
		for pidTid, _ := range prevPidCache {
			delete(prevPidCache, pidTid)
		}
	}
}

func BenchmarkPidSingleCache(b *testing.B) {
	for _, tc := range pidCacheTestCases {
		bfn := func(b *testing.B) {
			benchmarkPidSingleCache(b, tc.pidCount, tc.changedPidCount)
		}
		b.Run(
			fmt.Sprintf(
				"pidCount=%d,changedPidCount=%d",
				tc.pidCount, tc.changedPidCount,
			),
			bfn,
		)
	}
}

func BenchmarkPidDualCache(b *testing.B) {
	for _, tc := range pidCacheTestCases {
		bfn := func(b *testing.B) {
			benchmarkPidDualCache(b, tc.pidCount, tc.changedPidCount)
		}
		b.Run(
			fmt.Sprintf(
				"pidCount=%d,changedPidCount=%d",
				tc.pidCount, tc.changedPidCount,
			),
			bfn,
		)
	}
}
