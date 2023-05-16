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
	PrevStat          [2]*procfs.Stat2
	PrevCrtIndex      int
	PrevPCpu          map[int]*PCpuStat
	PrevTimestamp     *time.Time
	WantStat          [2]*procfs.Stat2
	WantCrtIndex      int
	WantPCpu          map[int]*PCpuStat
	Timestamp         time.Time
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
		return tc.Timestamp
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
	for i, stat := range tc.PrevStat {
		if stat == nil {
			procStatMetricsCtx.stat[i] = nil
		} else {
			procStatMetricsCtx.stat[i] = &procfs.Stat2{}
			*(procStatMetricsCtx.stat[i]) = *stat
		}
	}
	if tc.PrevPCpu == nil {
		procStatMetricsCtx.prevPCpu = nil
	} else {
		procStatMetricsCtx.prevPCpu = make(map[int]*PCpuStat)
		for cpu, pCpuStat := range tc.PrevPCpu {
			procStatMetricsCtx.prevPCpu[cpu] = &PCpuStat{}
			*(procStatMetricsCtx.prevPCpu[cpu]) = *pCpuStat
		}
	}
	procStatMetricsCtx.crtIndex = tc.PrevCrtIndex
	if tc.PrevTimestamp != nil {
		procStatMetricsCtx.Timestamp = *tc.PrevTimestamp
	}
	procStatMetricsCtx.refreshCycleNum = tc.RefreshCycleNum
	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procStatMetricsCtx.wChan = wChan
				procStatMetricsCtx.GenerateMetrics()
			},
			bufPool.ReturnBuffer,
		),
		true,
	)
	// Check buffer pool:
	wantBufPoolCount := int(0)
	gotBufPoolCount := bufPool.CheckedOutCount()
	if wantBufPoolCount != gotBufPoolCount {
		t.Errorf("bufPool.CheckedOutCount(): want %d, got %d", wantBufPoolCount, gotBufPoolCount)
	}
	var r testutils.DiffReporter
	if tc.WantCrtIndex != procStatMetricsCtx.crtIndex {
		t.Errorf("crtIndex mismatch: want %d, got %d", tc.WantCrtIndex, procStatMetricsCtx.crtIndex)
	}
	if !cmp.Equal(
		tc.WantStat,
		procStatMetricsCtx.stat,
		cmp.Reporter(&r),
	) {
		t.Errorf("stat mismatch (-want +got):\n%s", r.String())
	}
	if !cmp.Equal(
		tc.WantPCpu,
		procStatMetricsCtx.prevPCpu,
		cmpopts.EquateApprox(0, 1e-6),
		cmp.Reporter(&r),
	) {
		t.Errorf("prevPCpu mismatch (-want +got):\n%s", r.String())
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
