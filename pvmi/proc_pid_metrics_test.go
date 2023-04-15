// Verify compatibility between generated testcases and Go types

package pvmi

import (
	"bytes"
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

const (
	PID_METRICS_TEST_CASE_LOAD_FILE_NAME       = "proc_pid_metrics_test_case_load.json"
	PID_METRICS_TEST_CASES_FILE_NAME           = "proc_pid_metrics_test_cases.json"
	PID_METRICS_DELTA_TEST_CASES_FILE_NAME     = "proc_pid_metrics_delta_test_cases.json"
	ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_all_pid_metrics_delta_test_cases.json"
)

func TestPidMetricsTestCaseLoad(t *testing.T) {
	tc := PidMetricsTestCase{}

	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PID_METRICS_TEST_CASE_LOAD_FILE_NAME),
		&tc,
	)
	if err != nil {
		t.Fatal(err)
	}
	buf := &bytes.Buffer{}
	dumper := dump.NewDumper(buf, 3)
	dumper.NoColor = true
	dumper.ShowFlag = dump.Fnopos
	dumper.Dump(tc)
	t.Logf("%#v loaded into:\n%s\n", PID_METRICS_TEST_CASE_LOAD_FILE_NAME, buf.String())
}

func clonePmce(pmce *PidMetricsCacheEntry) *PidMetricsCacheEntry {
	new_pmce := &PidMetricsCacheEntry{
		PassNum:                 pmce.PassNum,
		FullMetricsCycleNum:     pmce.FullMetricsCycleNum,
		ProcStat:                &procfs.ProcStat{},
		ProcStatus:              &procfs.ProcStatus{},
		RawCgroup:               make([]byte, len(pmce.RawCgroup)),
		RawCmdline:              make([]byte, len(pmce.RawCmdline)),
		ProcIo:                  &procfs.ProcIO{},
		Timestamp:               pmce.Timestamp,
		CommonLabels:            pmce.CommonLabels,
		ProcPidCmdlineMetric:    pmce.ProcPidCmdlineMetric,
		ProcPidStatStateMetric:  pmce.ProcPidStatStateMetric,
		ProcPidStatInfoMetric:   pmce.ProcPidStatInfoMetric,
		ProcPidStatusInfoMetric: pmce.ProcPidStatusInfoMetric,
	}
	*(new_pmce.ProcStat) = *(pmce.ProcStat)
	*(new_pmce.ProcStatus) = *(pmce.ProcStatus)
	copy(new_pmce.RawCgroup, pmce.RawCgroup)
	copy(new_pmce.RawCmdline, pmce.RawCmdline)
	*(new_pmce.ProcIo) = *(pmce.ProcIo)
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
	pmtc PidMetricsTestCase,
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
		procfsRoot:        pmtc.ProcfsRoot,
		pmc:               pmc,
		psc:               psc,
		passNum:           pmtc.WantPmce.PassNum,
		fullMetricsFactor: pmtc.FullMetricsFactor,
		activeThreshold:   pmtc.ActiveThreshold,
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
	if diff := cmp.Diff(
		pmtc.WantPmce,
		gotPmce,
		cmpopts.IgnoreUnexported(PidMetricsCacheEntry{}),
		cmpopts.IgnoreUnexported(procfs.ProcStat{}),
		cmpopts.IgnoreUnexported(procfs.ProcStatus{}),
	); diff != "" {
		t.Errorf("mismatch (-want +got):\n%s", diff)
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
		addPidMetricsTestCase(&pmtc, pmc, tm, nil)
		psc := PidStarttimeCache{}

		prevPmceFullMetricsCycle := "N/A"
		if pmtc.PrevPmce != nil {
			prevPmceFullMetricsCycle = strconv.Itoa(pmtc.PrevPmce.FullMetricsCycleNum)
		}
		wantPmceFullMetricsCycle := "N/A"
		if pmtc.WantPmce != nil {
			wantPmceFullMetricsCycle = strconv.Itoa(pmtc.WantPmce.FullMetricsCycleNum)
		}
		t.Run(
			fmt.Sprintf(
				"Name=%s,Pid=%d,Tid=%d,FullMetricsFactor=%d,ActiveThreshold=%d,PmceFullMetricsCycle:%s->%s",
				pmtc.Name,
				pmtc.Pid,
				pmtc.Tid,
				pmtc.FullMetricsFactor,
				pmtc.ActiveThreshold,
				prevPmceFullMetricsCycle,
				wantPmceFullMetricsCycle,
			),
			func(t *testing.T) { runPidMetricsTestCase(t, pmtc, pmc, psc, tm) },
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
	procfsRoot := pmtcList[0].ProcfsRoot
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
			procfsRoot:        procfsRoot,
			pmc:               pmc,
			psc:               psc,
			passNum:           passNum,
			fullMetricsFactor: fullMetricsFactor,
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
