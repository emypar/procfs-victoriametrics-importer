// The scheduler is responsible for determining the next (the  nearest in time,
// that is) units of work that need to be done.
//
// A unit of work is represented by the pair (metricsGenFn, metricsGenCtx),
// which is added to the TODO Queue. MetricsGenFn are functions capable of
// parsing /proc stats into Prometheus exposition format metrics, using the
// context from metricsGenCtx. The latter is a container for configuration,
// state, stats, etc. and it is specific to the metrics generator. From the
// point of view of the scheduler, metricsGenCtx is an interface w/ a
// GetInterval method, which returns how ofter the metrics should be generated.
//
// The scheduler uses a min heap that stores the timestamps of future units of
// work and map which associates each timestamp with the list of units of work
// for that time.
//
// The scheduler writes units of work (metricsGenFn, metricsGenCtx) into a TODO
// queue, which is read by worker goroutines. The latter invoke
// metricsGenFn(metricsGenCtx) to generate the actual metrics.

package pvmi

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

const (
	SCHEDULER_TIMER_ID = "scheduler"
)

var SchedulerLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"Scheduler",
)

type TimeHeap []time.Time

func (h TimeHeap) Len() int { return len(h) }

func (h TimeHeap) Less(i, j int) bool {
	return h[i].Before(h[j])
}

func (h TimeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *TimeHeap) Push(t any) {
	*h = append(*h, t.(time.Time))
}

func (h *TimeHeap) Pop() any {
	old := *h
	n := len(*h)
	t := old[n-1]
	*h = old[0 : n-1]
	return t
}

type MetricsGenContext interface {
	GetInterval() time.Duration
}

type MetricsGenFn func(MetricsGenContext)

type MetricsWorkUnit struct {
	mGenFn  MetricsGenFn
	mGenCtx MetricsGenContext
}

type SchedulerCycleSync struct {
	// The test goroutine writes to this channel to start a scheduling cycle:
	start chan bool
	// The scheduler goroutine writes to this channel to indicate the end of the
	// scheduling cycle:
	done chan bool
}

type SchedulerContext struct {
	cond            *sync.Cond
	timeHeap        *TimeHeap
	timeWorkUnitMap map[time.Time][]*MetricsWorkUnit
	todo            chan *MetricsWorkUnit

	// The logistics for stopping the scheduler, needed during
	// testing:
	stopCtx      context.Context
	stopCancelFn context.CancelFunc
	stopWg       *sync.WaitGroup

	// Mocks during testing:
	timeNowFn  TimeNowFn
	newTimerFn NewMockableTimerFn

	// Test <-> scheduler sync channels:
	cycleSync *SchedulerCycleSync
}

func NewSchedulerContext(
	todo chan *MetricsWorkUnit,
	newTimerFn NewMockableTimerFn,
	timeNowFn TimeNowFn,
	cycleSync *SchedulerCycleSync,
) *SchedulerContext {
	if timeNowFn == nil {
		timeNowFn = time.Now
	}
	if newTimerFn == nil {
		newTimerFn = NewRealTimer
	}
	schedCtx := &SchedulerContext{
		cond:            sync.NewCond(&sync.Mutex{}),
		timeHeap:        &TimeHeap{},
		timeWorkUnitMap: make(map[time.Time][]*MetricsWorkUnit),
		todo:            todo,
		stopWg:          &sync.WaitGroup{},
		timeNowFn:       timeNowFn,
		newTimerFn:      newTimerFn,
		cycleSync:       cycleSync,
	}
	schedCtx.stopCtx, schedCtx.stopCancelFn = context.WithCancel(context.Background())
	heap.Init(schedCtx.timeHeap)
	return schedCtx
}

var GlobalSchedulerContext *SchedulerContext

func SetGlobalSchedulerContext(todo chan *MetricsWorkUnit) {
	GlobalSchedulerContext = NewSchedulerContext(todo, nil, nil, nil)
}

func (schedCtx *SchedulerContext) Start() {
	SchedulerLog.Info("Start scheduler")
	timer := schedCtx.newTimerFn(time.Hour, SCHEDULER_TIMER_ID)
	if !timer.Stop() {
		<-timer.GetChannel()
	}
	cycleSync := schedCtx.cycleSync
	schedCtx.stopWg.Add(1)

	// In order to be able to stop the scheduler before the 1st work unit is
	// added, the loop goroutine should reach the `Block until at least one work
	// unit is added' section. Use a simple channel to block this start function
	// until the goroutine writes to it, thus signaling that it is ready:s
	loopGoroutineInitialized := make(chan bool, 1)
	go func() {
		defer func() {
			schedCtx.stopWg.Done()
		}()

		// Block until at least one work unit is added:
		schedCtx.cond.L.Lock()
		loopGoroutineInitialized <- true
		for len(*schedCtx.timeHeap) == 0 {
			SchedulerLog.Info("Scheduler waiting for the first work unit")
			schedCtx.cond.Wait()
			select {
			case <-schedCtx.stopCtx.Done():
				SchedulerLog.Info("Scheduler cancelled")
				return
			default:
			}
		}
		schedCtx.cond.L.Unlock()

		SchedulerLog.Info("Scheduler loop started")
		for {
			// If invoked from a test goroutine, wait for signal to start a
			// scheduling cycle:
			if cycleSync != nil {
				<-cycleSync.start
			}
			timeNow := schedCtx.timeNowFn()
			nextWUTime := heap.Pop(schedCtx.timeHeap).(time.Time)
			pauseTillNextWUTime := nextWUTime.Sub(timeNow)
			if pauseTillNextWUTime < 0 {
				pauseTillNextWUTime = 0
			}
			timer.Reset(pauseTillNextWUTime)
			select {
			case <-schedCtx.stopCtx.Done():
				SchedulerLog.Info("Scheduler loop cancelled")
				if !timer.Stop() {
					<-timer.GetChannel()
				}
				return
			case <-timer.GetChannel():
			}
			schedCtx.cond.L.Lock()
			for _, wu := range schedCtx.timeWorkUnitMap[nextWUTime] {
				schedCtx.AddWU(wu)
			}
			delete(schedCtx.timeWorkUnitMap, nextWUTime)
			schedCtx.cond.L.Unlock()

			// If invoked from a test goroutine, signal cycle completion:
			if cycleSync != nil {
				cycleSync.done <- true
			}
		}
	}()
	<-loopGoroutineInitialized
	SchedulerLog.Info("Scheduler started")
}

func (schedCtx *SchedulerContext) Stop() {
	SchedulerLog.Info("Stopping the scheduler")
	schedCtx.cond.L.Lock()
	if len(*schedCtx.timeHeap) == 0 {
		schedCtx.cond.Broadcast()
	}
	schedCtx.cond.L.Unlock()
	schedCtx.stopCancelFn()
	schedCtx.stopWg.Wait()
	SchedulerLog.Info("Scheduler stopped")
}

func (schedCtx *SchedulerContext) Add(mGenFn MetricsGenFn, mGenCtx MetricsGenContext) {
	schedCtx.cond.L.Lock()
	wasEmpty := len(*schedCtx.timeHeap) == 0
	schedCtx.AddWU(&MetricsWorkUnit{mGenFn, mGenCtx})
	if wasEmpty {
		schedCtx.cond.Broadcast()
	}
	schedCtx.cond.L.Unlock()
}

func (schedCtx *SchedulerContext) AddWU(wu *MetricsWorkUnit) {
	// -> TODO:
	schedCtx.todo <- wu
	// The next time is the next multiple of the interval, in milliseconds:
	intervalMilliseconds := wu.mGenCtx.GetInterval().Milliseconds()
	timeNow := schedCtx.timeNowFn()
	nextWUTime := time.UnixMilli(
		(timeNow.UnixMilli()/intervalMilliseconds + 1) * intervalMilliseconds,
	)
	// Add to the list for that time (create it as needed):
	_, exists := schedCtx.timeWorkUnitMap[nextWUTime]
	if !exists {
		// New time, add it to the heap:
		heap.Push(schedCtx.timeHeap, nextWUTime)
		schedCtx.timeWorkUnitMap[nextWUTime] = make([]*MetricsWorkUnit, 0)
	}
	schedCtx.timeWorkUnitMap[nextWUTime] = append(
		schedCtx.timeWorkUnitMap[nextWUTime],
		wu,
	)
}
