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
// value is being tallied. The check is to verify that the actual rate
// sum(gotten credit)/runDuration is ~= replenishValue/replenishInt

const (
	TEST_CREDIT_MAX_RELATIVE_ERROR = 0.1
)

type stopFunc func()

type TestCreditContext struct {
	wg                     *sync.WaitGroup
	m                      *sync.Mutex
	c                      *Credit
	minDesired, maxDesired uint64
	stopList               []stopFunc
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

func (tcc *TestCreditContext) start(n int) {
	tcc.stopList = make([]stopFunc, n)
	for i := 0; i < n; i++ {
		tcc.stopList[i] = startCreditUser(tcc)
	}
}

func (tcc *TestCreditContext) stop() {
	for _, stopFn := range tcc.stopList {
		stopFn()
	}
	tcc.wg.Wait()
	tcc.c.StopReplenishWait()
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

func startCreditUser(tcc *TestCreditContext) stopFunc {
	ctx, cancelFunc := context.WithCancel(context.Background())
	c, minDesired, maxDesired := tcc.c, tcc.minDesired, tcc.maxDesired
	go func() {
		for {
			select {
			case <-ctx.Done():
				tcc.wg.Done()
				return
			default:
				desired := uint64(rand.Int63n(int64(maxDesired-minDesired))) + minDesired
				minAcceptable := uint64(rand.Int63n(int64(desired-1))) + 1
				got := c.GetCredit(desired, minAcceptable)
				tcc.m.Lock()
				tcc.usedCredit += got
				tcc.numCreditRequests += 1
				tcc.m.Unlock()
			}
		}
	}()
	tcc.wg.Add(1)

	return func() {
		cancelFunc()
	}
}

func testCredit(
	t *testing.T,
	tctc *TestCreditTestCase,
) {
	c := NewCredit(tctc.replenishValue, tctc.replenishValue, tctc.replenishInt)
	tcc := NewTestCreditContext(c, tctc.minDesired, tctc.maxDesired)
	tcc.start(tctc.numRequestors)
	testStart := time.Now()
	time.Sleep(tctc.testDuration)
	tcc.stop()
	actualDuration := time.Since(testStart)
	targetRate := float64(tctc.replenishValue) / (float64(tctc.replenishInt.Microseconds()) / 1_000_000.)
	actualRate := float64(tcc.usedCredit) / (float64(actualDuration.Microseconds()) / 1_000_000.)
	relativeError := math.Abs(actualRate-targetRate) / targetRate
	logFn := t.Logf
	if relativeError > TEST_CREDIT_MAX_RELATIVE_ERROR {
		logFn = t.Fatalf
	}
	logFn(
		"Req#: %d, rate: want: %.03f/sec, got: %.03f/sec, relativeError: want: <=%.02f, got: %.02f",
		tcc.numCreditRequests,
		targetRate, actualRate,
		TEST_CREDIT_MAX_RELATIVE_ERROR, relativeError,
	)
}

func TestCredit(t *testing.T) {
	for _, tctc := range []*TestCreditTestCase{
		{12_500, 100 * time.Millisecond, 1000, 10_000, 2, 2 * time.Second},
		{12_500, 100 * time.Millisecond, 1000, 10_000, 4, 2 * time.Second},
		{125_000, 100 * time.Millisecond, 1, 50_000, 16, 2 * time.Second},
	} {
		t.Run(
			fmt.Sprintf(
				"replenish=%d/%.06f,desired=%d..%d,numRequestors=%d,duration=%.06f",
				tctc.replenishValue, float64(tctc.replenishInt.Microseconds())/1_000_000.,
				tctc.minDesired, tctc.maxDesired,
				tctc.numRequestors,
				float64(tctc.testDuration.Microseconds())/1_000_000.,
			),
			func(t *testing.T) {
				testCredit(t, tctc)
			},
		)
	}
}
