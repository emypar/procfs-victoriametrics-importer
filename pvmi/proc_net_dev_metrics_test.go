// unit tests for proc_dev_net metrics

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

type ProcNetDevTestCase struct {
	Name                    string
	ProcfsRoot              string
	PrevNetDev              procfs.NetDev
	PrevTimestamp           int64
	PrevRefreshGroupNum     map[string]int64
	PrevNextRefreshGroupNum int64
	WantNetDev              procfs.NetDev
	Timestamp               int64
	WantRefreshGroupNum     map[string]int64
	WantMetrics             []string
	FullMetricsFactor       int64
	RefreshCycleNum         int64
	WantNextRefreshGroupNum int64
}

const (
	PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME       = "proc_net_dev_metrics_test_cases.json"
	PROC_NET_DEV_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_net_dev_metrics_delta_test_cases.json"
)

func procNetDevTest(t *testing.T, tc *ProcNetDevTestCase) {
	timeNow := func() time.Time {
		return time.UnixMilli(tc.Timestamp)
	}

	bufPool := NewBufferPool(256)
	procNetDevMetricsCtx, err := NewProcNetDevMetricsContext(
		time.Second,
		tc.FullMetricsFactor,
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
	procNetDevMetricsCtx.prevNetDev = tc.PrevNetDev
	procNetDevMetricsCtx.prevTs = time.UnixMilli(tc.PrevTimestamp)
	procNetDevMetricsCtx.refreshCycleNum = tc.RefreshCycleNum
	if tc.PrevRefreshGroupNum != nil {
		procNetDevMetricsCtx.refreshGroupNum = tc.PrevRefreshGroupNum
	}
	procNetDevMetricsCtx.nextRefreshGroupNum = tc.PrevNextRefreshGroupNum
	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procNetDevMetricsCtx.wChan = wChan
				GenerateProcNetDevMetrics(MetricsGenContext(procNetDevMetricsCtx))
			},
			bufPool.ReturnBuffer,
		),
		true,
	)
	// Visual checks:
	t.Logf("newDevices: %v\n", procNetDevMetricsCtx.newDevices)
	t.Logf("refreshGroupNum: %v\n", procNetDevMetricsCtx.refreshGroupNum)
	t.Logf("nextRefreshGroupNum: %v\n", procNetDevMetricsCtx.nextRefreshGroupNum)

	// Check buffer pool:
	wantBufPoolCount := int(0)
	gotBufPoolCount := bufPool.CheckedOutCount()
	if wantBufPoolCount != gotBufPoolCount {
		t.Errorf("bufPool.CheckedOutCount(): want %d, got %d", wantBufPoolCount, gotBufPoolCount)
	}
	// Note: certain fields are updated only for delta strategy:
	if tc.FullMetricsFactor > 1 {
		if diff := cmp.Diff(
			tc.WantNetDev,
			procNetDevMetricsCtx.prevNetDev,
		); diff != "" {
			t.Errorf("procfs.NetDev mismatch (-want +got):\n%s", diff)
		}
		if tc.WantRefreshGroupNum != nil {
			if diff := cmp.Diff(
				tc.WantRefreshGroupNum,
				procNetDevMetricsCtx.refreshGroupNum,
			); diff != "" {
				t.Errorf("refreshGroupNum mismatch (-want +got):\n%s", diff)
			}
		}
		if tc.WantNextRefreshGroupNum != procNetDevMetricsCtx.nextRefreshGroupNum {
			t.Errorf(
				"nextRefreshGroupNum: want: %d, got: %d",
				tc.WantNextRefreshGroupNum, procNetDevMetricsCtx.nextRefreshGroupNum,
			)
		}
	}
	wantMetrics := testutils.DuplicateStrings(tc.WantMetrics, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}

}

func TestProcNetDev(t *testing.T) {
	tcList := []ProcNetDevTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetDevTest(t, &tc) },
		)
	}
}

func TestProcNetDevDelta(t *testing.T) {
	tcList := []ProcNetDevTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_DEV_METRICS_DELTA_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetDevTest(t, &tc) },
		)
	}
}
