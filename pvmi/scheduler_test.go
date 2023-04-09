// Unit tests for the scheduler

package pvmi

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

const (
	TEST_SCHEDULER_MAX_TIME_MOCK_WAIT = time.Second
)

type SchedulerTestMGenCtx struct {
	interval time.Duration
	id       string
}

func (mGenCtx *SchedulerTestMGenCtx) GetInterval() time.Duration {
	return mGenCtx.interval
}

func SchedulerTestMetricsGenFn(mGenCtx MetricsGenContext) {
}

type SchedulerTestCase struct {
	mGenCtxList []*SchedulerTestMGenCtx
	wantIds     [][]string
}

func dumpSchedulerWorkUnitMap(schedCtx *SchedulerContext) string {
	nextTimeList := make(TimeHeap, len(*(schedCtx.timeHeap)))
	copy(nextTimeList, *(schedCtx.timeHeap))
	sort.Sort(nextTimeList)
	buf := &bytes.Buffer{}
	for i, nextTime := range nextTimeList {
		if i > 0 {
			buf.WriteString(", ")
		}
		idList := make([]string, len(schedCtx.timeWorkUnitMap[nextTime]))
		for i, wu := range schedCtx.timeWorkUnitMap[nextTime] {
			idList[i] = wu.mGenCtx.(*SchedulerTestMGenCtx).id
		}
		sort.Strings(idList)
		fmt.Fprintf(buf, "%s: [%s]", nextTime.UTC(), strings.Join(idList, ", "))
	}
	return buf.String()
}

func todoToIdList(todo chan *MetricsWorkUnit) []string {
	idList := make([]string, 0)
	done := false
	for !done {
		select {
		case wu := <-todo:
			idList = append(idList, wu.mGenCtx.(*SchedulerTestMGenCtx).id)
		default:
			done = true
		}
	}
	sort.Strings(idList)
	return idList
}

func testScheduler(t *testing.T, tc *SchedulerTestCase, startSchedFirst bool) {

	mockableTimers := testutils.NewMockTimePool()
	newTimerFn := func(d time.Duration, id string) MockableTimer {
		return MockableTimer(mockableTimers.NewMockTimer(d, id))
	}
	timeNow := time.UnixMilli(0)
	timeNowFn := func() time.Time { return timeNow }
	todo := make(chan *MetricsWorkUnit, len(tc.mGenCtxList))
	cycleSync := &SchedulerCycleSync{
		start: make(chan bool, 1),
		done:  make(chan bool, 1),
	}
	schedCtx := NewSchedulerContext(todo, newTimerFn, timeNowFn, cycleSync)

	schedStarted := false
	defer func() {
		if schedStarted {
			// Advance the scheduler goroutine to the timer, where it can be
			// stopped.
			cycleSync.start <- true
			schedCtx.Stop()
		}
	}()

	if startSchedFirst {
		schedCtx.Start()
		schedStarted = true
	}

	wantIds := tc.wantIds

	// Add the initial work unit list:
	cycleN := 0
	for _, mGenCtx := range tc.mGenCtxList {
		// Each add will make one call to timeNowFn:
		timeNow = time.UnixMilli(0)
		schedCtx.Add(SchedulerTestMetricsGenFn, MetricsGenContext(mGenCtx))
	}
	todoList := todoToIdList(todo)
	t.Logf("cycle# %d todo: [%s]", cycleN, strings.Join(todoList, ", "))
	t.Logf("cycle# %d pending: %s", cycleN, dumpSchedulerWorkUnitMap(schedCtx))
	if diff := cmp.Diff(wantIds[cycleN], todoList); diff != "" {
		t.Fatalf("WorkUnit Id mismatch (-want +got):\n%s", diff)
	}

	// Execute schedule steps for the rest of the want list:
	for cycleN++; cycleN < len(wantIds); cycleN++ {
		if !schedStarted {
			schedCtx.Start()
			schedStarted = true
		}
		// Assume everything happens on time, i.e. the time now is when the next
		// WU should be scheduled:
		timeNow = (*schedCtx.timeHeap)[0]

		// Start a scheduling cycle, it expects the timer to fire:
		cycleSync.start <- true
		err := mockableTimers.Fire(SCHEDULER_TIMER_ID, TEST_SCHEDULER_MAX_TIME_MOCK_WAIT)
		if err != nil {
			t.Fatal(err)
		}

		// Sync wait on the scheduler cycle:
		<-cycleSync.done

		// The cycle results:
		todoList := todoToIdList(todo)
		t.Logf("cycle# %d todo: [%s]", cycleN, strings.Join(todoList, ", "))
		t.Logf("cycle# %d pending: %s", cycleN, dumpSchedulerWorkUnitMap(schedCtx))
		if diff := cmp.Diff(wantIds[cycleN], todoList); diff != "" {
			t.Fatalf("WorkUnit Id mismatch (-want +got):\n%s", diff)
		}
	}

}

func SchedulerTestMGenCtxListToString(mGenCtxList []*SchedulerTestMGenCtx) string {
	buf := &bytes.Buffer{}
	for i, mGenCtx := range mGenCtxList {
		if i > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(buf, "(id=%s,intrv=%s)", mGenCtx.id, mGenCtx.interval)
	}
	return buf.String()
}

func SchedulerTestWantIdsToString(wantIds [][]string) string {
	buf := &bytes.Buffer{}
	for i, ids := range wantIds {
		if i > 0 {
			buf.WriteByte(',')
		}
		fmt.Fprintf(buf, "[%s]", strings.Join(ids, ","))
	}
	return buf.String()
}

func TestScheduler(t *testing.T) {
	for _, tc := range []*SchedulerTestCase{
		{
			mGenCtxList: []*SchedulerTestMGenCtx{
				{time.Second, "A"},
			},
			wantIds: [][]string{
				{"A"},
			},
		},
		{
			mGenCtxList: []*SchedulerTestMGenCtx{
				{time.Second, "A"},
				{3 * time.Second, "B"},
			},
			wantIds: [][]string{
				{"A", "B"},
			},
		},
		{
			mGenCtxList: []*SchedulerTestMGenCtx{
				{time.Second, "A"},
				{3 * time.Second, "B"},
			},
			wantIds: [][]string{
				{"A", "B"},
				{"A"}, {"A"}, {"A", "B"},
				{"A"}, {"A"}, {"A", "B"},
			},
		},
		{
			mGenCtxList: []*SchedulerTestMGenCtx{
				{100 * time.Millisecond, "A"},
				{200 * time.Millisecond, "B"},
				{300 * time.Millisecond, "C"},
			},
			wantIds: [][]string{
				{"A", "B", "C"},
				{"A"},
				{"A", "B"},
				{"A", "C"},
				{"A", "B"},
				{"A"},
				{"A", "B", "C"},
				{"A"},
				{"A", "B"},
				{"A", "C"},
				{"A", "B"},
				{"A"},
				{"A", "B", "C"},
			},
		},
	} {
		for i := 0; i < len(tc.wantIds); i++ {
			sort.Strings(tc.wantIds[i])
		}
		for _, startSchedFirst := range []bool{false, true} {
			t.Run(
				fmt.Sprintf(
					"startSchedFirst=%v,mGenCtxList=%s/wantIdList=%s",
					startSchedFirst,
					SchedulerTestMGenCtxListToString(tc.mGenCtxList),
					SchedulerTestWantIdsToString(tc.wantIds),
				),
				func(t *testing.T) { testScheduler(t, tc, startSchedFirst) },
			)
		}
	}
}

func TestSchedulerStopEmpty(y *testing.T) {
	todo := make(chan *MetricsWorkUnit, 1)
	schedCtx := NewSchedulerContext(todo, nil, nil, nil)
	schedCtx.Start()
	schedCtx.Stop()
}
