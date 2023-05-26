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

type ProcNetSnmpTestCase struct {
	Name              string
	ProcfsRoot        string
	PrevNetSnmp       *procfs.FixedLayoutDataModel
	PrevNetSnmp6      *procfs.FixedLayoutDataModel
	WantNetSnmp       *procfs.FixedLayoutDataModel
	WantNetSnmp6      *procfs.FixedLayoutDataModel
	Timestamp         time.Time
	WantMetrics       []string
	FullMetricsFactor int
	RefreshCycleNum   int
}

const (
	PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME        = "proc_net_snmp_metrics_test_cases.json"
	PROC_NET_SNMP_METRICS_DELTA_TEST_CASES_FILE_NAME  = "proc_net_snmp_metrics_delta_test_cases.json"
	PROC_NET_SNMP6_METRICS_TEST_CASES_FILE_NAME       = "proc_net_snmp6_metrics_test_cases.json"
	PROC_NET_SNMP6_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_net_snmp6_metrics_delta_test_cases.json"
)

func procNetSnmpSnmp6Test(t *testing.T, tc *ProcNetSnmpTestCase) {
	if tc.WantNetSnmp == nil && tc.WantNetSnmp6 == nil {
		return
	}

	timeNow := func() time.Time {
		return tc.Timestamp
	}

	bufPool := NewBufferPool(256)
	procNetSnmpMetricsCtx, err := NewProcNetSnmpMetricsContext(
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

	wantCrtNetSnmpIndex := 1 - procNetSnmpMetricsCtx.crtNetSnmpIndex
	procNetSnmpMetricsCtx.netSnmp[procNetSnmpMetricsCtx.crtNetSnmpIndex] = tc.PrevNetSnmp
	procNetSnmpMetricsCtx.skipNetSnmp = tc.WantNetSnmp == nil

	wantCrtNetSnmp6Index := 1 - procNetSnmpMetricsCtx.crtNetSnmp6Index
	procNetSnmpMetricsCtx.netSnmp6[procNetSnmpMetricsCtx.crtNetSnmp6Index] = tc.PrevNetSnmp6
	procNetSnmpMetricsCtx.skipNetSnmp6 = tc.WantNetSnmp6 == nil

	procNetSnmpMetricsCtx.refreshCycleNum = tc.RefreshCycleNum

	gotMetrics := testutils.DuplicateStrings(
		testutils.CollectMetrics(
			func(wChan chan *bytes.Buffer) {
				procNetSnmpMetricsCtx.wChan = wChan
				procNetSnmpMetricsCtx.GenerateMetrics()
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

	if tc.WantNetSnmp != nil {
		if wantCrtNetSnmpIndex != procNetSnmpMetricsCtx.crtNetSnmpIndex {
			t.Errorf(
				"crtNetSnmpIndex: want: %d, got: %d",
				wantCrtNetSnmpIndex, procNetSnmpMetricsCtx.crtNetSnmpIndex,
			)
		}

		var r testutils.DiffReporter

		if !cmp.Equal(
			tc.WantNetSnmp,
			procNetSnmpMetricsCtx.netSnmp[procNetSnmpMetricsCtx.crtNetSnmpIndex],
			cmp.Reporter(&r),
		) {
			t.Errorf("NetSnmp mismatch (-want +got):\n%s", r.String())
		}
	}

	if tc.WantNetSnmp6 != nil {
		if wantCrtNetSnmp6Index != procNetSnmpMetricsCtx.crtNetSnmp6Index {
			t.Errorf(
				"crtNetSnmp6Index: want: %d, got: %d",
				wantCrtNetSnmp6Index, procNetSnmpMetricsCtx.crtNetSnmp6Index,
			)
		}

		var r testutils.DiffReporter

		if !cmp.Equal(
			tc.WantNetSnmp6,
			procNetSnmpMetricsCtx.netSnmp6[procNetSnmpMetricsCtx.crtNetSnmp6Index],
			cmp.Reporter(&r),
		) {
			t.Errorf("NetSnmp6 mismatch (-want +got):\n%s", r.String())
		}
	}

	wantMetrics := testutils.DuplicateStrings(tc.WantMetrics, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}
}

func TestProcNetSnmp(t *testing.T) {
	tcList := []ProcNetSnmpTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetSnmpSnmp6Test(t, &tc) },
		)
	}
}

func TestProcNetSnmp6(t *testing.T) {
	tcList := []ProcNetSnmpTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_SNMP6_METRICS_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetSnmpSnmp6Test(t, &tc) },
		)
	}
}

func TestProcNetSnmpDelta(t *testing.T) {
	tcList := []ProcNetSnmpTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_SNMP_METRICS_DELTA_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetSnmpSnmp6Test(t, &tc) },
		)
	}
}

func TestProcNetSnmp6Delta(t *testing.T) {
	tcList := []ProcNetSnmpTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PROC_NET_SNMP6_METRICS_DELTA_TEST_CASES_FILE_NAME),
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
			func(t *testing.T) { procNetSnmpSnmp6Test(t, &tc) },
		)
	}
}
