// unit tests for proc_interrupt metrics

package pvmi

import (
	"bytes"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/prometheus/procfs"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

type ProcInterruptsTestCase struct {
	Name                    string
	ProcfsRoot              string
	PrevInterrupts          procfs.Interrupts
	PrevRefreshGroupNum     map[string]int
	PrevNextRefreshGroupNum int
	WantInterrupts          procfs.Interrupts
	Timestamp               time.Time
	WantRefreshGroupNum     map[string]int
	WantMetrics             []string
	FullMetricsFactor       int
	RefreshCycleNum         int
	WantNextRefreshGroupNum int
}

const (
	PROC_INTERRUPTS_METRICS_TEST_CASES_FILE_NAME       = "proc_interrupts_metrics_test_cases.json"
	PROC_INTERRUPTS_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_interrupts_metrics_delta_test_cases.json"
)

func procInterruptsTest(t *testing.T, tc *ProcInterruptsTestCase) {
	timeNow := func() time.Time {
		return tc.Timestamp
	}

	bufPool := NewBufferPool(256)
	procInterruptsMetricsCtx, err := NewProcInterruptsMetricsContext(
		time.Second,
		time.Second*time.Duration(tc.FullMetricsFactor),
		tc.ProcfsRoot,
		TestHostname,
		TestJob,
		timeNow,
		nil,
		bufPool,
	)
	if err != nil {
		t.Fatal(err)
	}
	procInterruptsMetricsCtx.skipSoftirqs = true
	prevCrtIndex := procInterruptsMetricsCtx.crtIndex
	procInterruptsMetricsCtx.interrupts[prevCrtIndex] = tc.PrevInterrupts
	procInterruptsMetricsCtx.refreshCycleNum = tc.RefreshCycleNum
	if tc.PrevRefreshGroupNum != nil {
		procInterruptsMetricsCtx.refreshGroupNum = tc.PrevRefreshGroupNum
	}
	procInterruptsMetricsCtx.nextRefreshGroupNum = tc.PrevNextRefreshGroupNum
	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procInterruptsMetricsCtx.wChan = wChan
				procInterruptsMetricsCtx.GenerateMetrics()
			},
			bufPool.ReturnBuffer,
		),
		true,
	)

	wantBufPoolCount := int(0)
	gotBufPoolCount := bufPool.CheckedOutCount()
	if wantBufPoolCount != gotBufPoolCount {
		t.Errorf("bufPool.CheckedOutCount(): want %d, got %d", wantBufPoolCount, gotBufPoolCount)
	}

	wantCrtIndex := 1 - prevCrtIndex
	if wantCrtIndex != procInterruptsMetricsCtx.crtIndex {
		t.Errorf(
			"crtIndex: want: %d, got: %d",
			wantCrtIndex, procInterruptsMetricsCtx.crtIndex,
		)
	}

	if tc.WantInterrupts != nil {
		if diff := cmp.Diff(
			tc.WantInterrupts,
			procInterruptsMetricsCtx.interrupts[procInterruptsMetricsCtx.crtIndex],
		); diff != "" {
			t.Errorf("procfs.Interrupts mismatch (-want +got):\n%s", diff)
		}
	}

	if tc.WantRefreshGroupNum != nil {
		if diff := cmp.Diff(
			tc.WantRefreshGroupNum,
			procInterruptsMetricsCtx.refreshGroupNum,
		); diff != "" {
			t.Errorf("refreshGroupNum mismatch (-want +got):\n%s", diff)
		}
	}

	if tc.WantNextRefreshGroupNum != procInterruptsMetricsCtx.nextRefreshGroupNum {
		t.Errorf(
			"nextRefreshGroupNum: want: %d, got: %d",
			tc.WantNextRefreshGroupNum, procInterruptsMetricsCtx.nextRefreshGroupNum,
		)
	}

	wantMetrics := testutils.DuplicateStrings(tc.WantMetrics, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}
}

func TestProcInterrupts(t *testing.T) {
	tcList := []ProcInterruptsTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_INTERRUPTS_METRICS_TEST_CASES_FILE_NAME),
		&tcList,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tcList {
		t.Run(
			fmt.Sprintf(
				"Name=%s,FullMetricsFactor=%d,RefreshCycleNum=%d",
				tc.Name, tc.FullMetricsFactor, tc.RefreshCycleNum,
			),
			func(t *testing.T) { procInterruptsTest(t, &tc) },
		)
	}
}

func TestProcInterruptsDelta(t *testing.T) {
	tcList := []ProcInterruptsTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_INTERRUPTS_METRICS_DELTA_TEST_CASES_FILE_NAME),
		&tcList,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range tcList {
		t.Run(
			fmt.Sprintf(
				"Name=%s,FullMetricsFactor=%d,RefreshCycleNum=%d",
				tc.Name, tc.FullMetricsFactor, tc.RefreshCycleNum,
			),
			func(t *testing.T) { procInterruptsTest(t, &tc) },
		)
	}
}
