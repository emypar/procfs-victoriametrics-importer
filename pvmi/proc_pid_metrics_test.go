// Verify compatibility between generated testcases and Go types

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"path"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/gookit/goutil/dump"

	"github.com/prometheus/procfs"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

const (
	PID_METRICS_TEST_CASE_LOAD_FILE_NAME       = "proc_pid_metrics_test_case_load.json"
	PID_METRICS_TEST_CASES_FILE_NAME           = "proc_pid_metrics_test_cases.json"
	PID_METRICS_DELTA_TEST_CASES_FILE_NAME     = "proc_pid_metrics_delta_test_cases.json"
	ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_all_pid_metrics_delta_test_cases.json"
)

var dumpTestCase = flag.Bool("dump-testcase", false, "Dump test case for error")

type PidMetricsTestCase struct {
	Name              string
	Pid               int
	Tid               int
	ProcfsRoot        string
	PrevPmce          *PidMetricsCacheEntry
	WantPmce          *PidMetricsCacheEntry
	WantMetrics       []string
	FullMetricsFactor int
	ActiveThreshold   uint
}

// Grouping criterion for GenerateAllPidMetrics, needed since all individual
// calls to GeneratePidMetrics share the context:
type PidMetricsTestCaseGroupKey struct {
	ProcfsRoot        string
	FullMetricsFactor int
	ActiveThreshold   uint
}

type PidMetricsTestCaseGroup map[PidMetricsTestCaseGroupKey][]PidMetricsTestCase

// time.Now() mock for testing:
//
// GeneratePidMetrics calls are made in the order of the test cases and each
// makes one call to time.Now() Additionally GenerateAllPidMetrics makes a first
// call to time.Now(), before invoking GeneratePidMetrics and it should receive
// min(test case timestamp).
type PidMetricsTestTimeNowMock struct {
	prometheusTs []int64
	i            int
	keepMin      bool
}

func (tm *PidMetricsTestTimeNowMock) timeNow() time.Time {
	t := time.UnixMilli(tm.prometheusTs[tm.i])
	tm.i += 1
	return t
}

func (tm *PidMetricsTestTimeNowMock) addTs(ts int64) {
	if tm.prometheusTs == nil {
		tm.prometheusTs = make([]int64, 0)
		tm.i = 0
	}
	if tm.keepMin {
		if len(tm.prometheusTs) == 0 {
			tm.prometheusTs = append(tm.prometheusTs, ts)
		} else if ts < tm.prometheusTs[0] {
			tm.prometheusTs[0] = ts
		}
	}
	tm.prometheusTs = append(tm.prometheusTs, ts)
}

func dumpPidMetricsTestCase(pmtc *PidMetricsTestCase) string {
	buf := &bytes.Buffer{}
	dumper := dump.NewDumper(buf, 3)
	dumper.NoColor = true
	dumper.ShowFlag = dump.Fnopos
	dumper.Dump(pmtc)
	return buf.String()
}

func clonePmce(pmce *PidMetricsCacheEntry) *PidMetricsCacheEntry {
	new_pmce := &PidMetricsCacheEntry{
		PassNum:                 pmce.PassNum,
		RefreshCycleNum:         pmce.RefreshCycleNum,
		CrtProcStatIndex:        pmce.CrtProcStatIndex,
		CrtProcStatusIndex:      pmce.CrtProcStatusIndex,
		CrtProcIoIndex:          pmce.CrtProcIoIndex,
		PrevUTimePct:            pmce.PrevUTimePct,
		PrevSTimePct:            pmce.PrevSTimePct,
		Timestamp:               pmce.Timestamp,
		CommonLabels:            pmce.CommonLabels,
		ProcPidCmdlineMetric:    pmce.ProcPidCmdlineMetric,
		ProcPidStatStateMetric:  pmce.ProcPidStatStateMetric,
		ProcPidStatInfoMetric:   pmce.ProcPidStatInfoMetric,
		ProcPidStatusInfoMetric: pmce.ProcPidStatusInfoMetric,
	}
	for i := 0; i < 2; i++ {
		if pmce.ProcStat[i] != nil {
			new_pmce.ProcStat[i] = new(procfs.ProcStat)
			*new_pmce.ProcStat[i] = *pmce.ProcStat[i]
		}
		if pmce.ProcStatus[i] != nil {
			new_pmce.ProcStatus[i] = new(procfs.ProcStatus)
			*new_pmce.ProcStatus[i] = *pmce.ProcStatus[i]
		}
		if pmce.ProcIo[i] != nil {
			new_pmce.ProcIo[i] = new(procfs.ProcIO)
			*new_pmce.ProcIo[i] = *pmce.ProcIo[i]
		}
	}
	if pmce.RawCgroup != nil {
		new_pmce.RawCgroup = make([]byte, len(pmce.RawCgroup), cap(pmce.RawCgroup))
		copy(new_pmce.RawCgroup, pmce.RawCgroup)
	}
	if pmce.RawCmdline != nil {
		new_pmce.RawCmdline = make([]byte, len(pmce.RawCmdline), cap(pmce.RawCmdline))
		copy(new_pmce.RawCmdline, pmce.RawCmdline)
	}
	if pmce.ProcPidCgroupMetrics != nil {
		new_pmce.ProcPidCgroupMetrics = map[string]bool{}
		for k, v := range pmce.ProcPidCgroupMetrics {
			new_pmce.ProcPidCgroupMetrics[k] = v
		}
	}
	return new_pmce
}

func addPidMetricsTestCase(
	pmtc *PidMetricsTestCase,
	pmc PidMetricsCache,
	tm *PidMetricsTestTimeNowMock,
	pidList *[]PidTidPair,
) {
	if pmtc.WantPmce != nil {
		// Expect GeneratePidMetrics call for this PID,TID:
		tm.addTs(pmtc.WantPmce.Timestamp)
		if pidList != nil {
			*pidList = append(*pidList, PidTidPair{pmtc.Pid, pmtc.Tid})
		}
	}
	if pmtc.PrevPmce != nil {
		// Seed the cache, as if this PID,TID was found in a previous pass:
		pmc[PidTidPair{pmtc.Pid, pmtc.Tid}] = clonePmce(pmtc.PrevPmce)
	}
}

// GeneratePidMetrics tests:
func runPidMetricsTestCase(
	t *testing.T,
	pmtc *PidMetricsTestCase,
	pmc PidMetricsCache,
	psc PidStarttimeCache,
	tm *PidMetricsTestTimeNowMock,
) {
	fs, err := procfs.NewFS(pmtc.ProcfsRoot)
	if err != nil {
		t.Error(err)
		return
	}
	buf := &bytes.Buffer{}

	// Generate new metrics:
	pidMetricsCtx := &PidMetricsContext{
		fs:                fs,
		pmc:               pmc,
		psc:               psc,
		passNum:           pmtc.WantPmce.PassNum,
		fullMetricsFactor: pmtc.FullMetricsFactor,
		activeThreshold:   pmtc.ActiveThreshold,
		fileBuf:           make([]byte, PROC_PID_FILE_BUFFER_MAX_LENGTH),
		hostname:          TestHostname,
		job:               TestJob,
		clktckSec:         TestClktckSec,
		timeNow:           tm.timeNow,
	}
	pidTid := PidTidPair{pmtc.Pid, pmtc.Tid}
	err = GeneratePidMetrics(pidTid, pidMetricsCtx, buf)
	if err != nil {
		t.Error(err)
		return
	}

	// Check the results:
	gotPmce := pmc[pidTid]
	if gotPmce == nil {
		t.Errorf("missing %T[%v] for %s", pmc, pidTid, t.Name())
		return
	}

	var r testutils.DiffReporter
	if !cmp.Equal(
		pmtc.WantPmce,
		gotPmce,
		cmpopts.IgnoreUnexported(PidMetricsCacheEntry{}),
		cmpopts.IgnoreUnexported(procfs.ProcStat{}),
		cmpopts.IgnoreUnexported(procfs.ProcStatus{}),
		cmp.Reporter(&r),
	) {
		if *dumpTestCase {
			t.Logf("%s\n", dumpPidMetricsTestCase(pmtc))
		}
		t.Errorf("Cache mismatch (-want +got):\n%s", r.String())
	}
	wantMetrics := testutils.DuplicateStrings(pmtc.WantMetrics, true)
	gotMetrics := testutils.BufToStrings(buf, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}
}

func runPidMetricsTestCases(
	t *testing.T,
	testCaseFile string,
) {
	pmtcList := []PidMetricsTestCase{}
	err := testutils.LoadJsonFile(testCaseFile, &pmtcList)
	if err != nil {
		t.Fatal(err)
	}

	tm := &PidMetricsTestTimeNowMock{keepMin: false}
	for _, pmtc := range pmtcList {
		pmc := PidMetricsCache{}
		psc := PidStarttimeCache{}

		prevPmceRefreshCycleNum := "N/A"
		if pmtc.PrevPmce != nil {
			prevPmceRefreshCycleNum = strconv.Itoa(pmtc.PrevPmce.RefreshCycleNum)
		}
		wantPmceRefreshCycleNum := "N/A"
		if pmtc.WantPmce != nil {
			wantPmceRefreshCycleNum = strconv.Itoa(pmtc.WantPmce.RefreshCycleNum)
		}
		t.Run(
			fmt.Sprintf(
				"Name=%s,Pid=%d,Tid=%d,FullMetricsFactor=%d,ActiveThreshold=%d,PmceRefreshCycleNum:%s->%s",
				pmtc.Name,
				pmtc.Pid,
				pmtc.Tid,
				pmtc.FullMetricsFactor,
				pmtc.ActiveThreshold,
				prevPmceRefreshCycleNum,
				wantPmceRefreshCycleNum,
			),
			func(t *testing.T) {
				addPidMetricsTestCase(&pmtc, pmc, tm, nil)
				runPidMetricsTestCase(t, &pmtc, pmc, psc, tm)
			},
		)
	}
}

// GeneratePidMetrics tests:
func TestPidMetricsTestCases(t *testing.T) {
	runPidMetricsTestCases(
		t,
		path.Join(TestdataTestCasesDir, PID_METRICS_TEST_CASES_FILE_NAME),
	)
}

func TestPidMetricsDeltaTestCases(t *testing.T) {
	runPidMetricsTestCases(
		t,
		path.Join(TestdataTestCasesDir, PID_METRICS_DELTA_TEST_CASES_FILE_NAME),
	)
}

// GenerateAllPidMetrics tests:
func runAllPidMetricsTestCases(
	t *testing.T,
	pmtcList []PidMetricsTestCase,
) {
	if len(pmtcList) == 0 {
		return
	}
	// All test cases have the same procfs root, full metrics factor and active
	// threshold, so we can use the 1st one to retrieve them:
	fs, err := procfs.NewFS(pmtcList[0].ProcfsRoot)
	if err != nil {
		t.Error(err)
		return
	}
	fullMetricsFactor, activeThreshold := pmtcList[0].FullMetricsFactor, pmtcList[0].ActiveThreshold

	tm := &PidMetricsTestTimeNowMock{keepMin: true}
	pmc := PidMetricsCache{}
	psc := PidStarttimeCache{}

	pidList := make([]PidTidPair, 0)
	getPids := func(nPart int) []PidTidPair {
		return pidList
	}

	passNum := uint64(42)
	wantPmtcList := make([]PidMetricsTestCase, 0)
	wantMetrics := make([]string, 0)
	for _, pmtc := range pmtcList {
		if pmtc.WantPmce != nil {
			// Make a copy to be able to change its passNum without altering the
			// original:
			pmtc.WantPmce = clonePmce(pmtc.WantPmce)
			pmtc.WantPmce.PassNum = passNum + 1
		}
		addPidMetricsTestCase(&pmtc, pmc, tm, &pidList)
		if pmtc.PrevPmce != nil {
			pmtc.PrevPmce.PassNum = passNum
		}
		wantPmtcList = append(wantPmtcList, pmtc)
		wantMetrics = append(wantMetrics, pmtc.WantMetrics...)
	}

	bufPool := NewBufferPool(256)
	mgf := func(wChan chan *bytes.Buffer) {
		mGenCtx := MetricsGenContext(&PidMetricsContext{
			fs:                fs,
			pmc:               pmc,
			psc:               psc,
			passNum:           passNum,
			fullMetricsFactor: fullMetricsFactor,
			fileBuf:           make([]byte, PROC_PID_FILE_BUFFER_MAX_LENGTH),
			hostname:          TestHostname,
			job:               TestJob,
			clktckSec:         TestClktckSec,
			timeNow:           tm.timeNow,
			getPids:           getPids,
			bufPool:           bufPool,
			wChan:             wChan,
			activeThreshold:   activeThreshold,
		})
		GenerateAllPidMetrics(mGenCtx)
	}
	gotMetrics := testutils.CollectMetrics(mgf, bufPool.ReturnBuffer)
	// Check buffer pool:
	wantBufPoolCount := int(0)
	gotBufPoolCount := bufPool.CheckedOutCount()
	if wantBufPoolCount != gotBufPoolCount {
		t.Errorf("bufPool.CheckedOutCount(): want %d, got %d", wantBufPoolCount, gotBufPoolCount)
	}

	// Check the current state in the cache:
	for _, pmtc := range wantPmtcList {
		pidTid := PidTidPair{pmtc.Pid, pmtc.Tid}
		wantPmce := pmtc.WantPmce
		if wantPmce == nil {
			continue
		}
		gotPmce := pmc[pidTid]
		if gotPmce == nil {
			t.Errorf("missing %T[%v] for %s", pmc, pidTid, t.Name())
		} else if diff := cmp.Diff(
			pmtc.WantPmce,
			gotPmce,
			cmpopts.IgnoreUnexported(PidMetricsCacheEntry{}),
			cmpopts.IgnoreUnexported(procfs.ProcStat{}),
			cmpopts.IgnoreUnexported(procfs.ProcStatus{}),
		); diff != "" {
			t.Errorf("mismatch (-want +got):\n%s", diff)
		}
		delete(pmc, pidTid)
	}
	// All that's left in cache is unexpected:
	for pidTid := range pmc {
		t.Errorf("unexpected %T[%v] for %s", pmc, pidTid, t.Name())
	}

	// Sort and compare the metrics:
	wantMetrics = testutils.DuplicateStrings(wantMetrics, true)
	gotMetrics = testutils.DuplicateStrings(gotMetrics, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}
}

func testAllPidMetricsTestCases(t *testing.T, testCaseFile string) {
	pmtcList := []PidMetricsTestCase{}
	err := testutils.LoadJsonFile(testCaseFile, &pmtcList)
	if err != nil {
		t.Fatal(err)
	}
	pmtcGroup := PidMetricsTestCaseGroup{}
	for _, pmtc := range pmtcList {
		pmtcGroupKey := PidMetricsTestCaseGroupKey{pmtc.ProcfsRoot, pmtc.FullMetricsFactor, pmtc.ActiveThreshold}
		_, exists := pmtcGroup[pmtcGroupKey]
		if !exists {
			pmtcGroup[pmtcGroupKey] = make([]PidMetricsTestCase, 0)
		}
		pmtcGroup[pmtcGroupKey] = append(pmtcGroup[pmtcGroupKey], pmtc)
	}
	for pmtcGroupKey, pmtcList := range pmtcGroup {
		t.Run(
			fmt.Sprintf(
				"ProcfsRoot=%s,FullMetricsFactor=%d,ActiveThreshold=%d",
				pmtcGroupKey.ProcfsRoot,
				pmtcGroupKey.FullMetricsFactor,
				pmtcGroupKey.ActiveThreshold,
			),
			func(t *testing.T) {
				runAllPidMetricsTestCases(t, pmtcList)
			},
		)
	}
}

func TestAllPidMetricsTestCases(t *testing.T) {
	testAllPidMetricsTestCases(t, path.Join(TestdataTestCasesDir, PID_METRICS_TEST_CASES_FILE_NAME))
}

func TestAllPidMetricsDeltaTestCases(t *testing.T) {
	testAllPidMetricsTestCases(t, path.Join(TestdataTestCasesDir, ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME))
}
