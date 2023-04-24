// unit tests for proc_stat_metrics

package pvmi

import (
	"bytes"
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/prometheus/procfs"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

// Test case timestamps are Prometheus timestamps, i.e. milliseconds since the epoch:
type ProcStatMetricsTestCase struct {
	Name              string
	ProcfsRoot        string
	PrevStat          *procfs.Stat
	PrevTimestamp     int64
	WantStat          *procfs.Stat
	Timestamp         int64
	WantMetrics       []string
	FullMetricsFactor int64
	RefreshCycleNum   int64
}

const (
	PROC_STAT_METRICS_TEST_CASES_FILE_NAME       = "proc_stat_metrics_test_cases.json"
	PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_stat_metrics_delta_test_cases.json"
)

func procStatTest(t *testing.T, tc *ProcStatMetricsTestCase) {
	timeNow := func() time.Time {
		return time.UnixMilli(tc.Timestamp)
	}

	bufPool := NewBufferPool(256)
	procStatMetricsCtx, err := NewProcStatMetricsContext(
		time.Second,
		tc.FullMetricsFactor,
		tc.ProcfsRoot,
		TestHostname,
		TestJob,
		TestClktckSec,
		timeNow,
		nil,
		bufPool,
	)
	if err != nil {
		t.Fatal(err)
	}
	procStatMetricsCtx.prevStat = tc.PrevStat
	procStatMetricsCtx.prevTs = time.UnixMilli(tc.PrevTimestamp)
	procStatMetricsCtx.refreshCycleNum = tc.RefreshCycleNum
	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procStatMetricsCtx.wChan = wChan
				GenerateProcStatMetrics(MetricsGenContext(procStatMetricsCtx))
			},
			bufPool.ReturnBuffer,
		),
		true,
	)
	//fmt.Printf("ctx.prevStat=%#v\n", procStatMetricsCtx.prevStat)
	// Check buffer pool:
	wantBufPoolCount := int(0)
	gotBufPoolCount := bufPool.CheckedOutCount()
	if wantBufPoolCount != gotBufPoolCount {
		t.Errorf("bufPool.CheckedOutCount(): want %d, got %d", wantBufPoolCount, gotBufPoolCount)
	}
	if diff := cmp.Diff(
		tc.WantStat,
		procStatMetricsCtx.prevStat,
		cmpopts.IgnoreFields(*tc.WantStat, "IRQ", "SoftIRQ"),
		cmpopts.EquateApprox(0, 1e-6),
	); diff != "" {
		t.Errorf("procfs.Stat mismatch (-want +got):\n%s", diff)
	}
	wantMetrics := testutils.DuplicateStrings(tc.WantMetrics, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}
}

func TestProcStat(t *testing.T) {
	tcList := []ProcStatMetricsTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_STAT_METRICS_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procStatTest(t, &tc) },
		)
	}
}

func TestProcStatDelta(t *testing.T) {
	tcList := []ProcStatMetricsTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procStatTest(t, &tc) },
		)
	}
}
