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
	"fmt"
	"sync"
	"time"
)

var SchedulerLog = Log.WithField(
	COMPONENT_FIELD_NAME,
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

type SchedulerContext struct {
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
}

func NewSchedulerContext(
	todo chan *MetricsWorkUnit,
	newTimerFn NewMockableTimerFn,
	timeNowFn TimeNowFn,
) *SchedulerContext {
	if timeNowFn == nil {
		timeNowFn = time.Now
	}
	if newTimerFn == nil {
		newTimerFn = NewRealTimer
	}
	schedCtx := &SchedulerContext{
		timeHeap:        &TimeHeap{},
		timeWorkUnitMap: make(map[time.Time][]*MetricsWorkUnit),
		todo:            todo,
		stopWg:          &sync.WaitGroup{},
		timeNowFn:       timeNowFn,
		newTimerFn:      newTimerFn,
	}
	schedCtx.stopCtx, schedCtx.stopCancelFn = context.WithCancel(context.Background())
	heap.Init(schedCtx.timeHeap)
	return schedCtx
}

var GlobalSchedulerContext *SchedulerContext

func SetGlobalSchedulerContext(todo chan *MetricsWorkUnit) {
	GlobalSchedulerContext = NewSchedulerContext(todo, nil, nil)
}

func (schedCtx *SchedulerContext) Start() error {
	if len(*schedCtx.timeHeap) == 0 {
		return fmt.Errorf("Empty schedule, cannot start")
	}

	SchedulerLog.Info("Start Scheduler")
	schedCtx.stopWg.Add(1)
	go func() {
		defer func() {
			schedCtx.stopWg.Done()
		}()

		timer := schedCtx.newTimerFn(time.Hour, "scheduler")
		if !timer.Stop() {
			<-timer.GetChannel()
		}
		for {
			nextWUTime := heap.Pop(schedCtx.timeHeap).(time.Time)
			pauseTillNextWUTime := time.Until(nextWUTime)
			if pauseTillNextWUTime < 0 {
				pauseTillNextWUTime = 0
			}
			timer.Reset(pauseTillNextWUTime)
			select {
			case <-schedCtx.stopCtx.Done():
				if !timer.Stop() {
					<-timer.GetChannel()
				}
				return
			case <-timer.GetChannel():
			}
			for _, WU := range schedCtx.timeWorkUnitMap[nextWUTime] {
				schedCtx.AddWU(WU)
			}
		}
	}()
	return nil
}

func (schedCtx *SchedulerContext) Stop() {
	schedCtx.stopCancelFn()
	schedCtx.stopWg.Wait()
	SchedulerLog.Info("Scheduler stopped")
}

func (schedCtx *SchedulerContext) Add(mGenFn MetricsGenFn, mGenCtx MetricsGenContext) {
	schedCtx.AddWU(&MetricsWorkUnit{mGenFn, mGenCtx})
}

func (schedCtx *SchedulerContext) AddWU(WU *MetricsWorkUnit) {
	// -> TODO:
	schedCtx.todo <- WU
	// The next time is the next multiple of the interval, in milliseconds:
	intervalMilliseconds := WU.mGenCtx.GetInterval().Milliseconds()
	nextWUTime := time.UnixMilli(
		(schedCtx.timeNowFn().UnixMilli()/intervalMilliseconds + 1) * intervalMilliseconds,
	)
	// Add to the list for that time (create it as needed):
	_, exists := schedCtx.timeWorkUnitMap[nextWUTime]
	if !exists {
		// New time, add it to the heap
		heap.Push(schedCtx.timeHeap, nextWUTime)
		schedCtx.timeWorkUnitMap[nextWUTime] = make([]*MetricsWorkUnit, 0)
	}
	schedCtx.timeWorkUnitMap[nextWUTime] = append(
		schedCtx.timeWorkUnitMap[nextWUTime],
		WU,
	)
}
