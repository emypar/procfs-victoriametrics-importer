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
	PrevRxBps, PrevTxBps    NetDevBps
	PrevTimestamp           time.Time
	PrevRefreshGroupNum     NetDevRefreshGroupNum
	PrevNextRefreshGroupNum int64
	WantNetDev              procfs.NetDev
	WantRxBps, WantTxBps    NetDevBps
	Timestamp               time.Time
	WantRefreshGroupNum     NetDevRefreshGroupNum
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
		return tc.Timestamp
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
	if tc.PrevNetDev != nil {
		procNetDevMetricsCtx.prevNetDev = make(procfs.NetDev)
		for device, deviceLine := range tc.PrevNetDev {
			procNetDevMetricsCtx.prevNetDev[device] = deviceLine
		}
	}
	if tc.PrevRxBps != nil {
		for device, bps := range tc.PrevRxBps {
			procNetDevMetricsCtx.prevRxBps[device] = bps
		}
	}
	if tc.PrevTxBps != nil {
		for device, bps := range tc.PrevRxBps {
			procNetDevMetricsCtx.prevTxBps[device] = bps
		}
	}

	procNetDevMetricsCtx.prevTs = tc.PrevTimestamp
	procNetDevMetricsCtx.refreshCycleNum = tc.RefreshCycleNum
	if tc.PrevRefreshGroupNum != nil {
		procNetDevMetricsCtx.refreshGroupNum = tc.PrevRefreshGroupNum
	}
	procNetDevMetricsCtx.nextRefreshGroupNum = tc.PrevNextRefreshGroupNum
	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procNetDevMetricsCtx.wChan = wChan
				procNetDevMetricsCtx.GenerateMetrics()
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

	var r testutils.DiffReporter

	if !cmp.Equal(
		tc.WantNetDev,
		procNetDevMetricsCtx.prevNetDev,
		cmp.Reporter(&r),
	) {
		t.Errorf("procfs.NetDev mismatch (-want +got):\n%s", r.String())
	}

	if tc.WantRxBps != nil && !cmp.Equal(
		tc.WantRxBps,
		procNetDevMetricsCtx.prevRxBps,
		cmp.Reporter(&r),
	) {
		t.Errorf("prevRxBps mismatch (-want +got):\n%s", r.String())
	}

	if tc.WantTxBps != nil && !cmp.Equal(
		tc.WantTxBps,
		procNetDevMetricsCtx.prevTxBps,
		cmp.Reporter(&r),
	) {
		t.Errorf("prevTxBps mismatch (-want +got):\n%s", r.String())
	}

	// Note: certain fields are updated only for delta strategy:
	if tc.FullMetricsFactor > 1 {
		if tc.WantRefreshGroupNum != nil {
			if !cmp.Equal(
				tc.WantRefreshGroupNum,
				procNetDevMetricsCtx.refreshGroupNum,
				cmp.Reporter(&r),
			) {
				t.Errorf("refreshGroupNum mismatch (-want +got):\n%s", r.String())
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
