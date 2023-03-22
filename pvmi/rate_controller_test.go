// Tests for the credit mechanism

package pvmi

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"
)

// Run N credit requestors for a given duration D, each asking in a loop for a
// random credit between minDesired, maxDesired. Upon receiving the credit, its
// value is being tallied. Verify that the actual rate sum(gotten
// credit)/runDuration is ~= replenishValue/replenishInt; the actual test
// abs(gotRate - targetRate) / targetRate <= relativeError threshold.

const (
	TEST_CREDIT_MAX_RELATIVE_ERROR = 0.2
)

type stopFunc func()

type TestCreditContext struct {
	wg                     *sync.WaitGroup
	m                      *sync.Mutex
	c                      *Credit
	minDesired, maxDesired uint64
	stopFnList             []stopFunc
	usedCredit             uint64
	numCreditRequests      uint64
}

type TestCreditTestCase struct {
	replenishValue         uint64
	replenishInt           time.Duration
	minDesired, maxDesired uint64
	numRequestors          int
	testDuration           time.Duration
}

func (tcCtx *TestCreditContext) start(n int) {
	tcCtx.stopFnList = make([]stopFunc, n)
	for i := 0; i < n; i++ {
		tcCtx.stopFnList[i] = startCreditUser(tcCtx)
	}
}

func (tcCtx *TestCreditContext) stop() {
	for _, stopFn := range tcCtx.stopFnList {
		stopFn()
	}
	tcCtx.wg.Wait()
	tcCtx.c.StopReplenishWait()
}

func NewTestCreditContext(c *Credit, minDesired, maxDesired uint64) *TestCreditContext {
	return &TestCreditContext{
		wg:         &sync.WaitGroup{},
		m:          &sync.Mutex{},
		c:          c,
		minDesired: minDesired,
		maxDesired: maxDesired,
	}
}

func startCreditUser(tcCtx *TestCreditContext) stopFunc {
	ctx, cancelFunc := context.WithCancel(context.Background())
	c, minDesired, maxDesired := tcCtx.c, tcCtx.minDesired, tcCtx.maxDesired
	go func() {
		for {
			select {
			case <-ctx.Done():
				tcCtx.wg.Done()
				return
			default:
				desired := uint64(rand.Int63n(int64(maxDesired-minDesired))) + minDesired
				minAcceptable := uint64(rand.Int63n(int64(desired-1))) + 1
				got := c.GetCredit(desired, minAcceptable)
				tcCtx.m.Lock()
				tcCtx.usedCredit += got
				tcCtx.numCreditRequests += 1
				tcCtx.m.Unlock()
			}
		}
	}()
	tcCtx.wg.Add(1)

	return func() {
		cancelFunc()
	}
}

func testCredit(
	t *testing.T,
	testCase *TestCreditTestCase,
) {
	c := NewCredit(testCase.replenishValue, testCase.replenishValue, testCase.replenishInt)
	tcCtx := NewTestCreditContext(c, testCase.minDesired, testCase.maxDesired)
	tcCtx.start(testCase.numRequestors)
	testStart := time.Now()
	time.Sleep(testCase.testDuration)
	tcCtx.stop()
	actualDuration := time.Since(testStart)
	targetRate := float64(testCase.replenishValue) / (float64(testCase.replenishInt.Microseconds()) / 1_000_000.)
	actualRate := float64(tcCtx.usedCredit) / (float64(actualDuration.Microseconds()) / 1_000_000.)
	relativeError := math.Abs(actualRate-targetRate) / targetRate
	logFn := t.Logf
	if relativeError > TEST_CREDIT_MAX_RELATIVE_ERROR {
		logFn = t.Fatalf
	}
	logFn(
		"Req#: %d, rate: want: %.03f/sec, got: %.03f/sec, relativeError: want: <=%.02f, got: %.02f",
		tcCtx.numCreditRequests,
		targetRate, actualRate,
		TEST_CREDIT_MAX_RELATIVE_ERROR, relativeError,
	)
}

func TestCredit(t *testing.T) {
	for _, testCase := range []*TestCreditTestCase{
		{12_500, 100 * time.Millisecond, 1000, 10_000, 2, 2 * time.Second},
		{12_500, 100 * time.Millisecond, 1000, 10_000, 4, 2 * time.Second},
		{125_000, 100 * time.Millisecond, 1, 50_000, 16, 2 * time.Second},
	} {
		t.Run(
			fmt.Sprintf(
				"replenish=%d/%.06f,desired=%d..%d,numRequestors=%d,duration=%.06f",
				testCase.replenishValue, float64(testCase.replenishInt.Microseconds())/1_000_000.,
				testCase.minDesired, testCase.maxDesired,
				testCase.numRequestors,
				float64(testCase.testDuration.Microseconds())/1_000_000.,
			),
			func(t *testing.T) {
				testCredit(t, testCase)
			},
		)
	}
}
