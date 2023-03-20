package pvmi

import (
	"fmt"
	"path"
	"testing"
	"time"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

const (
	PID_LIST_TEST_VALID_DURATION = time.Second
	PID_LIST_TEST_CASE_FILE_NAME = "pid_list_test_case.json"
)

type PidListTestCase struct {
	ProcfsRoot string
	flags      uint32
	PidTidList [][2]int
}

func testPidListCache(
	t *testing.T,
	nPart int,
	tc *PidListTestCase,
) {

	pidListCache := NewPidListCache(nPart, PID_LIST_TEST_VALID_DURATION, tc.ProcfsRoot, tc.flags)
	for part := 0; part < nPart; part++ {
		want := make(map[PidTidPair]bool)
		for _, pidTidArray := range tc.PidTidList {
			pidTid := PidTidPair{pidTidArray[0], pidTidArray[1]}
			if (pidListCache.IsEnabledFor(PID_LIST_CACHE_PID_ENABLED_FLAG) &&
				pidTid.tid == 0 && (pidTid.pid%nPart == part)) ||
				(pidListCache.IsEnabledFor(PID_LIST_CACHE_TID_ENABLED_FLAG) &&
					pidTid.tid > 0 && pidTid.tid%nPart == part) {
				want[pidTid] = true
			}
		}

		pidList := pidListCache.GetPids(part)
		if pidList == nil {
			t.Errorf("%s: no list for  part %d", t.Name(), part)
		}
		for _, pidTid := range pidList {
			_, exists := want[pidTid]
			if exists {
				delete(want, pidTid)
			} else {
				t.Errorf("%s: unexpected pidTid %v for part %d", t.Name(), pidTid, part)
			}
		}
		for pidTid, _ := range want {
			t.Errorf("%s: missing pidTid %v for part %d", t.Name(), pidTid, part)
		}
	}
}

func TestPidListCache(t *testing.T) {
	tc := PidListTestCase{}
	err := testutils.LoadJsonFile(path.Join(TestdataTestCasesDir, PID_LIST_TEST_CASE_FILE_NAME), &tc)

	if err != nil {
		t.Fatal(err)
	}
	for nPart := 1; nPart <= 16; nPart++ {
		flags_list := []uint32{
			0,
			PID_LIST_CACHE_PID_ENABLED_FLAG,
			PID_LIST_CACHE_TID_ENABLED_FLAG,
			PID_LIST_CACHE_ALL_ENABLED_FLAG | PID_LIST_CACHE_TID_ENABLED_FLAG,
		}
		for _, flags := range flags_list {
			tc.flags = flags
			pidEnabled := tc.flags&PID_LIST_CACHE_PID_ENABLED_FLAG > 0
			tidEnabled := tc.flags&PID_LIST_CACHE_TID_ENABLED_FLAG > 0
			t.Run(
				fmt.Sprintf("nPart=%d,pidEnabled=%v,tidEnabled=%v", nPart, pidEnabled, tidEnabled),
				func(t *testing.T) {
					testPidListCache(t, nPart, &tc)
				},
			)
		}
	}
}
