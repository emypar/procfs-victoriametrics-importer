// proc/<pid> metrics

// To make the PID metrics generation more efficient, the following concepts are
// applied:
//
//  * at each scan, PIDs will be classified as active/inactive, based on delta
//  CPU from the previous scan compared against a threshold T. `status`, `io`,
//  etc. files will be read only for active PIDs.
//
//  * for all but the Nth scan, metrics will be generated only for values
//  changed from the previous scan; the Nth scan is a full metrics scan based
//  on all files.
//
//  * a full metrics cycle counter is maintained for each PID: 1..N->1..N, etc.
//  When its value == N, the scan is considered full metrics for that PID. To
//  stagger full metrics cycles across all PIDs, the counter is seeded with a
//  different value at PID creation. For instance the seed itself could cycle
//  1..N with every use (or it could be a random choice in the 1..N range).
//
// The behavior above is controlled by 2 configurable parameters: the active
// threshold T and full metrics factor N. Each can be disabled independently by
// choosing T = 0 or N = 1.

package pvmi

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	// Labels common to all proc_pid_* metrics:
	PROC_PID_METRIC_PID_LABEL_NAME         = "pid"
	PROC_PID_METRIC_STARTTIME_LABEL_NAME   = "starttime"
	PROC_PID_METRIC_TID_LABEL_NAME         = "tid"
	PROC_PID_METRIC_T_STARTTIME_LABEL_NAME = "t_starttime"

	// The max length of the buffer used to read files like cmdline and cgroup:
	PROC_PID_FILE_BUFFER_MAX_LENGTH = 0x10000

	// Stats metrics:
	PROC_PID_METRICS_UP_GROUP_NAME                         = "proc_pid_metrics"
	PROC_PID_METRICS_UP_INTERVAL_LABEL_NAME                = "interval"
	PROC_PID_METRICS_STATS_PID_COUNT_METRICS_NAME          = "proc_pid_metrics_stats_pid_count"
	PROC_PID_METRICS_STATS_TID_COUNT_METRICS_NAME          = "proc_pid_metrics_stats_tid_count"
	PROC_PID_METRICS_STATS_ACTIVE_COUNT_METRICS_NAME       = "proc_pid_metrics_stats_active_count"
	PROC_PID_METRICS_STATS_FULL_METRICS_COUNT_METRICS_NAME = "proc_pid_metrics_stats_full_metrics_count"
	PROC_PID_METRICS_STATS_CREATED_COUNT_METRICS_NAME      = "proc_pid_metrics_stats_created_count"
	PROC_PID_METRICS_STATS_DELETED_COUNT_METRICS_NAME      = "proc_pid_metrics_stats_deleted_count"
	PROC_PID_METRICS_STATS_ERROR_COUNT_METRICS_NAME        = "proc_pid_metrics_stats_error_count"
	PROC_PID_METRICS_STATS_GENERATED_COUNT_METRICS_NAME    = "proc_pid_metrics_stats_generated_count"
	PROC_PID_METRICS_STATS_CACHE_COUNT_METRICS_NAME        = "proc_pid_metrics_stats_cache_count"
	PROC_PID_METRICS_STATS_BYTES_COUNT_METRICS_NAME        = "proc_pid_metrics_stats_bytes_count"
	PROC_PID_METRICS_STATS_PID_LIST_PART_LABEL_NAME        = "pid_list_part"
)

var PidMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"PidMetrics",
)

var pidMetricCommonLabelsFmt = fmt.Sprintf(
	`%s="%%s",%s="%%s",%s="%%d",%s="%%d"`,
	HOSTNAME_LABEL_NAME, JOB_LABEL_NAME, PROC_PID_METRIC_PID_LABEL_NAME, PROC_PID_METRIC_STARTTIME_LABEL_NAME,
)
var tidMetricCommonLabelsFmt = pidMetricCommonLabelsFmt + "," + fmt.Sprintf(
	`%s="%%d",%s="%%d"`,
	PROC_PID_METRIC_TID_LABEL_NAME, PROC_PID_METRIC_T_STARTTIME_LABEL_NAME,
)

type PidMetricsStats struct {
	PidCount         uint64
	TidCount         uint64
	ActiveCount      uint64
	FullMetricsCount uint64
	CreatedCount     uint64
	DeletedCount     uint64
	ErrorCount       uint64
	GeneratedCount   uint64
	PmcCount         uint64
	BytesCount       uint64
}

// The context for pid metrics generation:
type PidMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// The part#, if the PID,TID space is partitioned among goroutines:
	pidListPart int
	// The metrics cache:
	pmc PidMetricsCache
	// The starttime cache:
	psc PidStarttimeCache
	// Full metrics factor, N. For delta strategy, metrics are generated only
	// for stats that changed from the previous run, however the full set is
	// generated at every Nth pass. Use  N = 1 for full metrics every time.
	fullMetricsFactor int
	// Full metrics will be generated for a PID[,TID] when its refreshCycleNum
	// == 0. The latter is a counter % fullMetricsFactor, which is initialized
	// when the process/thread is discovered. To stagger the full metrics cycles
	// across all PIDs, the counter is initialized from a seed which is
	// incremented, also % fullMetricsFactor, after each use.
	refreshCycleNumSeed int
	// Active process threshold. A process/thread is deemed active if its delta(UTime
	// + STime) >= threshold; for a non-active, non-full metrics pass, only the
	// `stat` based metrics are being generated. Use threshold = 0 to disable it.
	activeThreshold uint
	// The pass#:
	passNum uint64
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// Stats struct:
	pmStats *PidMetricsStats
	// A buffer for reading files that are cached in raw format, like cmdline
	// and cgroup. Such files do not change very often so the typical paradigm
	// is read, compare against cached (previous) and discard because no change.
	// That makes the buffer reusable across processes/threads.
	fileBuf []byte
	// The following are useful for testing (mocks, special behavior):
	enableStatsMetrics bool
	hostname           string
	job                string
	clktckSec          float64
	timeNow            TimeNowFn
	getPids            func(int) []PidTidPair
	bufPool            *BufferPool
}

func (pidMetricsCtx *PidMetricsContext) GetInterval() time.Duration {
	return pidMetricsCtx.interval
}

func NewPidMetricsContext(
	interval time.Duration,
	procfsRoot string,
	pidListPart int,
	fullMetricsFactor int,
	activeThreshold uint,
	// needed for testing:
	enableStatsMetrics bool,
	hostname string,
	job string,
	clktckSec float64,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	getPids GetPidsFn,
	bufPool *BufferPool,
) (*PidMetricsContext, error) {
	if hostname == "" {
		hostname = GlobalMetricsHostname
	}
	if job == "" {
		job = GlobalMetricsJob
	}
	if clktckSec <= 0 {
		clktckSec = ClktckSec
	}
	if timeNow == nil {
		timeNow = time.Now
	}
	if wChan == nil {
		wChan = GlobalMetricsWriteChannel
	}
	if getPids == nil {
		getPids = GlobalPidListCache.GetPids
	}
	if bufPool == nil {
		bufPool = GlobalBufPool
	}
	fs, err := procfs.NewFS(procfsRoot)
	if err != nil {
		return nil, err
	}
	pidMetricsCtx := &PidMetricsContext{
		interval:           interval,
		fs:                 fs,
		pidListPart:        pidListPart,
		pmc:                PidMetricsCache{},
		psc:                PidStarttimeCache{},
		fullMetricsFactor:  fullMetricsFactor,
		activeThreshold:    activeThreshold,
		passNum:            0,
		wChan:              wChan,
		fileBuf:            make([]byte, PROC_PID_FILE_BUFFER_MAX_LENGTH),
		enableStatsMetrics: enableStatsMetrics,
		hostname:           hostname,
		job:                job,
		clktckSec:          clktckSec,
		timeNow:            timeNow,
		getPids:            getPids,
		bufPool:            bufPool,
	}
	return pidMetricsCtx, nil
}

func (pidMetricsCtx *PidMetricsContext) ClearPidStarttimeCache() {
	pidMetricsCtx.psc = PidStarttimeCache{}
}

func (pidMetricsCtx *PidMetricsContext) ClearStats() {
	pidMetricsCtx.pmStats = &PidMetricsStats{}
}

func (pidMetricsCtx *PidMetricsContext) GetStats() *PidMetricsStats {
	return pidMetricsCtx.pmStats
}

func (pidMetricsCtx *PidMetricsContext) GetRefreshCycleNumSeed() int {
	refreshCycleNumSeed := pidMetricsCtx.refreshCycleNumSeed
	pidMetricsCtx.refreshCycleNumSeed += 1
	if pidMetricsCtx.refreshCycleNumSeed >= pidMetricsCtx.fullMetricsFactor {
		pidMetricsCtx.refreshCycleNumSeed = 0
	}
	return refreshCycleNumSeed
}

// Previous pass cache, required for delta strategy, pseudo-categorical metrics
// and for %CPU.
type PidMetricsCacheEntry struct {
	// The pass# when this entry was updated, needed for detecting out-of-scope
	// PIDs:
	PassNum uint64
	// Refresh cycle#, see refreshCycleNumSeed in PidMetricsContext:
	RefreshCycleNum int
	// In order to avoid constant memory allocation and garbage collection use
	// the [2]*Object dual buffer approach for the cached information. Each pass
	// will have a pair of crt, prev indices (crt in {0,1}, prev=1-crt) flipped
	// at the beginning of the pass. The new state is read into Object[crt] and
	// the previous state is available in Object[prev=1-crt].
	ProcStat           [2]*procfs.ProcStat
	CrtProcStatIndex   int
	ProcStatus         [2]*procfs.ProcStatus
	CrtProcStatusIndex int
	ProcIo             [2]*procfs.ProcIO
	CrtProcIoIndex     int
	// Cache raw data from /proc for info that is expensive to parse and which
	// doesn't change often:
	RawCgroup  []byte
	RawCmdline []byte
	// Cache %CPU time, needed for delta strategy. If either of the % changes
	// then all 3 (system, user, total) are being generated. A negative value
	// stands for invalid (this information is available at the end of the 2nd
	// scan).
	PrevUTimePct, PrevSTimePct float64
	// Stats acquisition time stamp:
	Timestamp time.Time
	// Cache common labels:
	CommonLabels string
	// Pseudo-categorical metrics; useful to cache them since most likely they
	// do not change from one pass to the next:
	ProcPidCgroupMetrics    map[string]bool
	ProcPidCmdlineMetric    string
	ProcPidStatStateMetric  string
	ProcPidStatInfoMetric   string
	ProcPidStatusInfoMetric string
}

// The cache is indexed by (PID, TID):
type PidMetricsCache map[PidTidPair]*PidMetricsCacheEntry

// Cache the starttime of PIDs, needed for TIDs common labels. The cache is
// valid for the entire pass and the idea is that multiple threads belonging to
// the same PID may be discovered in that pass.
type PidStarttimeCache map[int]uint64

func (psc PidStarttimeCache) resolvePidStarttime(
	pid int,
	fs procfs.FS,
	pmc PidMetricsCache,
) (uint64, error) {
	// Cached?
	starttime, ok := psc[pid]
	if ok {
		return starttime, nil
	}
	// PID in pmc? Glean it from there:
	//  - first as a process:
	pmce := pmc[PidTidPair{pid: pid, tid: 0}]
	if pmce == nil {
		// - next, try as the main thread:
		pmce = pmc[PidTidPair{pid: pid, tid: pid}]
	}
	if pmce != nil && pmce.ProcStat[1-pmce.CrtProcStatIndex] != nil {
		starttime = pmce.ProcStat[1-pmce.CrtProcStatIndex].Starttime
	} else {
		// Last resort, straight from /proc:
		proc, err := fs.Proc(pid)
		if err != nil {
			return 0, err
		}
		procStat, err := proc.Stat()
		if err != nil {
			return 0, err
		}
		starttime = procStat.Starttime
	}
	// Update starttime cache for further use:
	psc[pid] = starttime

	return starttime, nil
}

// Generate proc_pid_metrics_... for a given PID or TID.
//
// Args:
//
//	pidTid: (PID, TID) pair, TID == 0 for PID only
//	pidMetricsCtx: PidMetricsContext
//	fullMetrics: bool
//	buf: the buffer to write metrics into
//
// Return value:
//
//	The error condition, if any
func GeneratePidMetrics(
	pidTid PidTidPair,
	pidMetricsCtx *PidMetricsContext,
	buf *bytes.Buffer,
) error {
	var (
		proc                       procfs.Proc
		procStat, prevProcStat     *procfs.ProcStat
		savedCrtProcStatIndex      int
		procStatus, prevProcStatus *procfs.ProcStatus
		savedCrtProcStatusIndex    int
		procIo, prevProcIo         *procfs.ProcIO
		savedCrtProcIoIndex        int
		err                        error
		generatedCount             uint64
		deltaUTime, deltaSTime     uint
	)

	pmc := pidMetricsCtx.pmc
	psc := pidMetricsCtx.psc
	pmStats := pidMetricsCtx.pmStats

	pid, tid := pidTid.pid, pidTid.tid
	isForPid := tid == 0
	isActive, fullMetrics, prevDeleted := false, pidMetricsCtx.fullMetricsFactor <= 1, false

	// Keep track of changed pseudo-categorical metrics, needed to generate
	// metrics on updates only.
	cgroupChanged := false
	cmdlineChanged := false
	statStateChanged := false
	statInfoChanged := false
	statusInfoChanged := false

	// Cache entry: new v. update:
	pmce, exists := pmc[pidTid]
	bufInitLen := buf.Len()

	if exists {
		savedCrtProcStatIndex = pmce.CrtProcStatIndex
		savedCrtProcStatusIndex = pmce.CrtProcStatusIndex
		savedCrtProcIoIndex = pmce.CrtProcIoIndex
	}

	// On return:
	//  - update stats
	//  - update/restore state depending on error
	defer func() {
		if pmStats != nil {
			if isForPid {
				pmStats.PidCount += 1
			} else {
				pmStats.TidCount += 1
			}
			if err == nil {
				if isActive {
					pmStats.ActiveCount += 1
				}
				if fullMetrics {
					pmStats.FullMetricsCount += 1
				}
				if !exists {
					pmStats.CreatedCount += 1
				}
				pmStats.GeneratedCount += generatedCount
			} else {
				pmStats.ErrorCount += 1
			}
			if prevDeleted {
				pmStats.DeletedCount += 1
			}
		}
		if err == nil {
			pmce.PassNum = pidMetricsCtx.passNum
			if !exists && pmce != nil {
				pmc[pidTid] = pmce
			}
		} else {
			// Restore the current index for dual buffers:
			if exists && pmce != nil {
				pmce.CrtProcStatIndex = savedCrtProcStatIndex
				pmce.CrtProcStatusIndex = savedCrtProcStatusIndex
				pmce.CrtProcIoIndex = savedCrtProcIoIndex
			}
			// Discard metrics generated thus far:
			if buf.Len() > bufInitLen {
				buf.Truncate(bufInitLen)
			}
		}
	}()

	if isForPid {
		proc, err = pidMetricsCtx.fs.Proc(pid)
	} else {
		proc, err = pidMetricsCtx.fs.Thread(pid, tid)
	}
	if err != nil {
		return err
	}

	procDir := proc.Dir()

	if !exists {
		pmce = &PidMetricsCacheEntry{
			CrtProcStatIndex:   1,
			CrtProcStatusIndex: 1,
			CrtProcIoIndex:     1,
			PrevUTimePct:       -1.,
			PrevSTimePct:       -1.,
		}
		if pidMetricsCtx.fullMetricsFactor > 1 {
			pmce.RefreshCycleNum = pidMetricsCtx.GetRefreshCycleNumSeed()
		}
	}

	prevProcStat = pmce.ProcStat[pmce.CrtProcStatIndex]
	pmce.CrtProcStatIndex = 1 - pmce.CrtProcStatIndex
	procStat = pmce.ProcStat[pmce.CrtProcStatIndex]
	if procStat == nil {
		procStat = &procfs.ProcStat{}
		pmce.ProcStat[pmce.CrtProcStatIndex] = procStat
	}
	_, err = proc.StatWithStruct(procStat)
	if err != nil {
		return err
	}
	if exists && procStat.Starttime != prevProcStat.Starttime {
		// A different instance of the same PID/TID, generate clear metrics for
		// the previous one's pseudo-categorical ones:
		clearProcPidMetrics(
			pmce,
			strconv.FormatInt(pidMetricsCtx.timeNow().UnixMilli(), 10),
			buf,
			&generatedCount,
		)
		// Readjust the buffer recovery mark to accommodate the above:
		bufInitLen = buf.Len()
		// Clear cached pseudo-categorical metrics
		pmce.CommonLabels = ""
		pmce.ProcPidCgroupMetrics = nil
		pmce.ProcPidCmdlineMetric = ""
		pmce.ProcPidStatInfoMetric = ""
		pmce.ProcPidStatStateMetric = ""
		pmce.ProcPidStatusInfoMetric = ""
		exists = false
	}

	if exists {
		// Check process active status:
		deltaUTime = procStat.UTime - prevProcStat.UTime
		deltaSTime = procStat.STime - prevProcStat.STime
		if pidMetricsCtx.activeThreshold == 0 || (deltaUTime+deltaSTime) >= pidMetricsCtx.activeThreshold {
			isActive = true
		}
		// Check full refresh cycle:
		if pidMetricsCtx.fullMetricsFactor > 1 {
			fullMetrics = pmce.RefreshCycleNum == 0
			pmce.RefreshCycleNum += 1
			if pmce.RefreshCycleNum >= pidMetricsCtx.fullMetricsFactor {
				pmce.RefreshCycleNum = 0
			}
		}
	} else {
		fullMetrics = true
	}

	if isActive || fullMetrics {
		prevProcStatus = pmce.ProcStatus[pmce.CrtProcStatusIndex]
		pmce.CrtProcStatusIndex = 1 - pmce.CrtProcStatusIndex
		procStatus = pmce.ProcStatus[pmce.CrtProcStatusIndex]
		if procStatus == nil {
			procStatus = &procfs.ProcStatus{}
			pmce.ProcStatus[pmce.CrtProcStatusIndex] = procStatus
		}
		_, err = proc.NewStatusWithStruct(procStatus)
		if err != nil {
			return err
		}

		prevProcIo = pmce.ProcIo[pmce.CrtProcIoIndex]
		pmce.CrtProcIoIndex = 1 - pmce.CrtProcIoIndex
		procIo = pmce.ProcIo[pmce.CrtProcIoIndex]
		if procIo == nil {
			procIo = &procfs.ProcIO{}
			pmce.ProcIo[pmce.CrtProcIoIndex] = procIo
		}
		_, err = proc.IOWithStruct(procIo)
		if err != nil {
			return err
		}
	}
	if fullMetrics {
		fileBuf := pidMetricsCtx.fileBuf

		readRawFile := func(path string, dst *[]byte) (bool, error) {
			f, err := os.Open(path)
			if err != nil {
				return false, fmt.Errorf("%s: %s", path, err)
			}
			b := fileBuf[:cap(fileBuf)]
			n, err := f.Read(b)
			f.Close()
			if err != nil && err != io.EOF {
				return false, fmt.Errorf("%s: %s", path, err)
			}
			b = b[:n]
			if bytes.Equal(b, *dst) && *dst != nil {
				return false, nil
			}
			if len(*dst) < n || *dst == nil {
				*dst = make([]byte, n)
			}
			copy(*dst, b)
			*dst = (*dst)[:n]
			return true, nil
		}

		cgroupChanged, err = readRawFile(path.Join(procDir, "cgroup"), &pmce.RawCgroup)
		if err != nil {
			return err
		}
		cmdlineChanged, err = readRawFile(path.Join(procDir, "cmdline"), &pmce.RawCmdline)
		if err != nil {
			return err
		}
	}

	timestamp := pidMetricsCtx.timeNow().UTC()
	promTs := strconv.FormatInt(timestamp.UnixMilli(), 10)

	if !exists {
		if isForPid {
			pmce.CommonLabels = fmt.Sprintf(
				pidMetricCommonLabelsFmt,
				pidMetricsCtx.hostname,
				pidMetricsCtx.job,
				procStat.PID,
				procStat.Starttime,
			)
		} else {
			starttime, err := psc.resolvePidStarttime(pid, pidMetricsCtx.fs, pmc)
			if err != nil {
				return err
			}
			pmce.CommonLabels = fmt.Sprintf(
				tidMetricCommonLabelsFmt,
				pidMetricsCtx.hostname,
				pidMetricsCtx.job,
				pid,
				starttime,
				procStat.PID,
				procStat.Starttime,
			)
		}
		err = updateProcPidCgroupMetric(
			pmce,
			promTs,
			buf,
			&generatedCount,
		)
		if err != nil {
			return err
		}
		updateProcPidCmdlineMetric(pmce, promTs, buf, &generatedCount)
		updateProcPidStatStateMetric(pmce, promTs, buf, &generatedCount)
		updateProcPidStatInfoMetric(pmce, promTs, buf, &generatedCount)
		updateProcPidStatusInfoMetric(pmce, promTs, buf, &generatedCount)
	} else {
		// Check for changes in pseudo-categorical metrics:
		if prevProcStat.State != procStat.State {
			updateProcPidStatStateMetric(pmce, promTs, buf, &generatedCount)
			statStateChanged = true
		}
		if prevProcStat.Comm != procStat.Comm ||
			prevProcStat.PPID != procStat.PPID ||
			prevProcStat.PGRP != procStat.PGRP ||
			prevProcStat.Session != procStat.Session ||
			prevProcStat.TTY != procStat.TTY ||
			prevProcStat.TPGID != procStat.TPGID ||
			prevProcStat.Flags != procStat.Flags ||
			prevProcStat.Priority != procStat.Priority ||
			prevProcStat.Nice != procStat.Nice ||
			prevProcStat.RTPriority != procStat.RTPriority ||
			prevProcStat.Policy != procStat.Policy {
			updateProcPidStatInfoMetric(pmce, promTs, buf, &generatedCount)
			statInfoChanged = true
		}

		if isActive || fullMetrics {
			if prevProcStatus.Name != procStatus.Name ||
				prevProcStatus.TGID != procStatus.TGID ||
				prevProcStatus.UIDs != procStatus.UIDs ||
				prevProcStatus.GIDs != procStatus.GIDs {
				updateProcPidStatusInfoMetric(pmce, promTs, buf, &generatedCount)
				statusInfoChanged = true
			}
		}

		if cgroupChanged {
			err = updateProcPidCgroupMetric(
				pmce,
				promTs,
				buf,
				&generatedCount,
			)
			if err != nil {
				return err
			}
		}
		if cmdlineChanged {
			updateProcPidCmdlineMetric(pmce, promTs, buf, &generatedCount)
		}
	}

	// Generate pseudo-categorical metrics; they all share the same value
	// timestamp suffix:
	setSuffix := []byte(" 1 " + promTs + "\n")

	if fullMetrics || cgroupChanged {
		for metric, _ := range pmce.ProcPidCgroupMetrics {
			buf.WriteString(metric)
			buf.Write(setSuffix)
			generatedCount += 1
		}
	}

	if fullMetrics || cmdlineChanged {
		buf.WriteString(pmce.ProcPidCmdlineMetric)
		buf.Write(setSuffix)
		generatedCount += 1
	}

	if fullMetrics || statStateChanged {
		buf.WriteString(pmce.ProcPidStatStateMetric)
		buf.Write(setSuffix)
		generatedCount += 1
	}

	if fullMetrics || statInfoChanged {
		buf.WriteString(pmce.ProcPidStatInfoMetric)
		buf.Write(setSuffix)
		generatedCount += 1
	}

	if fullMetrics || statusInfoChanged {
		buf.WriteString(pmce.ProcPidStatusInfoMetric)
		buf.Write(setSuffix)
		generatedCount += 1
	}

	// Update numerical metrics:
	commonLabels := pmce.CommonLabels

	// proc_pid_stat_minflt:
	if fullMetrics || procStat.MinFlt != prevProcStat.MinFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_MINFLT_METRIC_NAME,
			commonLabels,
			procStat.MinFlt,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cminflt:
	if fullMetrics || procStat.CMinFlt != prevProcStat.CMinFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CMINFLT_METRIC_NAME,
			commonLabels,
			procStat.CMinFlt,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_majflt:
	if fullMetrics || procStat.MajFlt != prevProcStat.MajFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_MAJFLT_METRIC_NAME,
			commonLabels,
			procStat.MajFlt,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cmajflt:
	if fullMetrics || procStat.CMajFlt != prevProcStat.CMajFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CMAJFLT_METRIC_NAME,
			commonLabels,
			procStat.CMajFlt,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_utime_seconds:
	if fullMetrics || procStat.UTime != prevProcStat.UTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_UTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.UTime)*pidMetricsCtx.clktckSec,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_stime_seconds:
	if fullMetrics || procStat.STime != prevProcStat.STime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_STIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.STime)*pidMetricsCtx.clktckSec,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cutime_seconds:
	if fullMetrics || procStat.CUTime != prevProcStat.CUTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_CUTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.CUTime)*pidMetricsCtx.clktckSec,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cstime_seconds:
	if fullMetrics || procStat.CSTime != prevProcStat.CSTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_CSTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.CSTime)*pidMetricsCtx.clktckSec,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_vsize:
	if fullMetrics || procStat.VSize != prevProcStat.VSize {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_VSIZE_METRIC_NAME,
			commonLabels,
			procStat.VSize,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_rss:
	if fullMetrics || procStat.RSS != prevProcStat.RSS {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_RSS_METRIC_NAME,
			commonLabels,
			procStat.RSS,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_rsslim:
	if fullMetrics || procStat.RSSLimit != prevProcStat.RSSLimit {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_RSSLIM_METRIC_NAME,
			commonLabels,
			procStat.RSSLimit,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cpu:
	if fullMetrics || procStat.Processor != prevProcStat.Processor {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CPU_METRIC_NAME,
			commonLabels,
			procStat.Processor,
			promTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_*time_pct:
	if exists {
		// % = delta(XTime) * clktckSec / delta(time) * 100
		//                    ( <------ pCpuFactor ------> )
		pCpuFactor := pidMetricsCtx.clktckSec / timestamp.Sub(pmce.Timestamp).Seconds() * 100.
		UTimePct, STimePct := float64(deltaUTime)*pCpuFactor, float64(deltaSTime)*pCpuFactor
		// Note: when comparing %, use the same precision as that used for display (2DP):
		if fullMetrics ||
			int(pmce.PrevUTimePct*100.) != int(UTimePct*100.) ||
			int(pmce.PrevSTimePct*100.) != int(STimePct*100.) {
			fmt.Fprintf(
				buf,
				"%s{%s} %.02f %s\n%s{%s} %.02f %s\n%s{%s} %.02f %s\n",
				PROC_PID_STAT_UTIME_PCT_METRIC_NAME,
				commonLabels,
				UTimePct,
				promTs,
				PROC_PID_STAT_STIME_PCT_METRIC_NAME,
				commonLabels,
				STimePct,
				promTs,
				PROC_PID_STAT_CPU_TIME_PCT_METRIC_NAME,
				commonLabels,
				UTimePct+STimePct,
				promTs,
			)
			generatedCount += 3
			pmce.PrevUTimePct, pmce.PrevSTimePct = UTimePct, STimePct
		}
	}

	if isActive || fullMetrics {
		if isForPid {
			// proc_pid_status_vm_peak:
			if fullMetrics || procStatus.VmPeak != prevProcStatus.VmPeak {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_PEAK_METRIC_NAME,
					commonLabels,
					procStatus.VmPeak,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_size:
			if fullMetrics || procStatus.VmSize != prevProcStatus.VmSize {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_SIZE_METRIC_NAME,
					commonLabels,
					procStatus.VmSize,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_lck:
			if fullMetrics || procStatus.VmLck != prevProcStatus.VmLck {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_LCK_METRIC_NAME,
					commonLabels,
					procStatus.VmLck,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_pin:
			if fullMetrics || procStatus.VmPin != prevProcStatus.VmPin {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_PIN_METRIC_NAME,
					commonLabels,
					procStatus.VmPin,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_hwm:
			if fullMetrics || procStatus.VmHWM != prevProcStatus.VmHWM {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_HWM_METRIC_NAME,
					commonLabels,
					procStatus.VmHWM,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_rss:
			if fullMetrics || procStatus.VmRSS != prevProcStatus.VmRSS {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_RSS_METRIC_NAME,
					commonLabels,
					procStatus.VmRSS,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_rss_anon:
			if fullMetrics || procStatus.RssAnon != prevProcStatus.RssAnon {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_RSS_ANON_METRIC_NAME,
					commonLabels,
					procStatus.RssAnon,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_rss_file:
			if fullMetrics || procStatus.RssFile != prevProcStatus.RssFile {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_RSS_FILE_METRIC_NAME,
					commonLabels,
					procStatus.RssFile,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_rss_shmem:
			if fullMetrics || procStatus.RssShmem != prevProcStatus.RssShmem {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_RSS_SHMEM_METRIC_NAME,
					commonLabels,
					procStatus.RssShmem,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_data:
			if fullMetrics || procStatus.VmData != prevProcStatus.VmData {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_DATA_METRIC_NAME,
					commonLabels,
					procStatus.VmData,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_stk:
			if fullMetrics || procStatus.VmStk != prevProcStatus.VmStk {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_STK_METRIC_NAME,
					commonLabels,
					procStatus.VmStk,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_exe:
			if fullMetrics || procStatus.VmExe != prevProcStatus.VmExe {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_EXE_METRIC_NAME,
					commonLabels,
					procStatus.VmExe,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_lib:
			if fullMetrics || procStatus.VmLib != prevProcStatus.VmLib {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_LIB_METRIC_NAME,
					commonLabels,
					procStatus.VmLib,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_pte:
			if fullMetrics || procStatus.VmPTE != prevProcStatus.VmPTE {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_PTE_METRIC_NAME,
					commonLabels,
					procStatus.VmPTE,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_pmd:
			if fullMetrics || procStatus.VmPMD != prevProcStatus.VmPMD {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_PMD_METRIC_NAME,
					commonLabels,
					procStatus.VmPMD,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_vm_swap:
			if fullMetrics || procStatus.VmSwap != prevProcStatus.VmSwap {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_VM_SWAP_METRIC_NAME,
					commonLabels,
					procStatus.VmSwap,
					promTs,
				)
				generatedCount += 1
			}

			// proc_pid_status_hugetbl_pages:
			if fullMetrics || procStatus.HugetlbPages != prevProcStatus.HugetlbPages {
				fmt.Fprintf(
					buf,
					"%s{%s} %d %s\n",
					PROC_PID_STATUS_HUGETBL_PAGES_METRIC_NAME,
					commonLabels,
					procStatus.HugetlbPages,
					promTs,
				)
				generatedCount += 1
			}

		}

		// proc_pid_status_voluntary_ctxt_switches:
		if fullMetrics || procStatus.VoluntaryCtxtSwitches != prevProcStatus.VoluntaryCtxtSwitches {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VOLUNTARY_CTXT_SWITCHES_METRIC_NAME,
				commonLabels,
				procStatus.VoluntaryCtxtSwitches,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_nonvoluntary_ctxt_switches:
		if fullMetrics || procStatus.NonVoluntaryCtxtSwitches != prevProcStatus.NonVoluntaryCtxtSwitches {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_NONVOLUNTARY_CTXT_SWITCHES_METRIC_NAME,
				commonLabels,
				procStatus.NonVoluntaryCtxtSwitches,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_rcar:
		if fullMetrics || procIo.RChar != prevProcIo.RChar {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_RCAR_METRIC_NAME,
				commonLabels,
				procIo.RChar,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_wcar:
		if fullMetrics || procIo.WChar != prevProcIo.WChar {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_WCAR_METRIC_NAME,
				commonLabels,
				procIo.WChar,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_syscr:
		if fullMetrics || procIo.SyscR != prevProcIo.SyscR {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_SYSCR_METRIC_NAME,
				commonLabels,
				procIo.SyscR,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_syscw:
		if fullMetrics || procIo.SyscW != prevProcIo.SyscW {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_SYSCW_METRIC_NAME,
				commonLabels,
				procIo.SyscW,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_readbytes:
		if fullMetrics || procIo.ReadBytes != prevProcIo.ReadBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_READBYTES_METRIC_NAME,
				commonLabels,
				procIo.ReadBytes,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_writebytes:
		if fullMetrics || procIo.WriteBytes != prevProcIo.WriteBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_WRITEBYTES_METRIC_NAME,
				commonLabels,
				procIo.WriteBytes,
				promTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_cancelled_writebytes:
		if fullMetrics || procIo.CancelledWriteBytes != prevProcIo.CancelledWriteBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_CANCELLED_WRITEBYTES_METRIC_NAME,
				commonLabels,
				procIo.CancelledWriteBytes,
				promTs,
			)
			generatedCount += 1
		}
	}

	if generatedCount > 0 {
		buf.WriteByte('\n')
	}

	// Update the cache entry:
	pmce.Timestamp = timestamp

	err = nil
	return err
}

// Clear pseudo-categorical metrics for out-of-scope PID:
func clearProcPidMetrics(
	pmce *PidMetricsCacheEntry,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	gCnt := uint64(0)
	clearSuffix := []byte(" 0 " + promTs + "\n")
	for metric, _ := range pmce.ProcPidCgroupMetrics {
		buf.WriteString(metric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if pmce.ProcPidCmdlineMetric != "" {
		buf.WriteString(pmce.ProcPidCmdlineMetric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if pmce.ProcPidStatStateMetric != "" {
		buf.WriteString(pmce.ProcPidStatStateMetric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if pmce.ProcPidStatInfoMetric != "" {
		buf.WriteString(pmce.ProcPidStatInfoMetric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if pmce.ProcPidStatusInfoMetric != "" {
		buf.WriteString(pmce.ProcPidStatusInfoMetric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if generatedCount != nil {
		*generatedCount += gCnt
	}
	return nil
}

// One pass for all PID[,TIDs] assigned to this scanner:
func GenerateAllPidMetrics(mGenCtx MetricsGenContext) {
	pidMetricsCtx := mGenCtx.(*PidMetricsContext)
	// Pass timestamp:
	timestamp := pidMetricsCtx.timeNow().UnixMilli()

	// Pass init:
	pidMetricsCtx.passNum += 1
	bufPool, wChan, passNum := pidMetricsCtx.bufPool, pidMetricsCtx.wChan, pidMetricsCtx.passNum

	// Get the pid list:
	pidList := pidMetricsCtx.getPids(pidMetricsCtx.pidListPart)

	// Iterate through the (sorted) list and generate metrics:
	pidMetricsCtx.ClearPidStarttimeCache()
	pidMetricsCtx.ClearStats()
	buf := bufPool.GetBuffer()
	bytesCount := 0
	for _, pidTid := range pidList {
		err := GeneratePidMetrics(pidTid, pidMetricsCtx, buf)
		if err != nil {
			PidMetricsLog.Warnf("GeneratePidMetrics(%v): %s", pidTid, err)
		}
		if buf.Len() >= BUF_MAX_SIZE {
			bytesCount += buf.Len()
			if wChan != nil {
				wChan <- buf
			} else {
				bufPool.ReturnBuffer(buf)
			}
			buf = bufPool.GetBuffer()
		}
	}

	// Clear expired PIDs:
	promTs := strconv.FormatInt(timestamp, 10)
	dCnt, pmcCnt := uint64(0), uint64(0)
	pmc := pidMetricsCtx.pmc
	for pidTid, pmce := range pmc {
		if pmce.PassNum != passNum {
			clearProcPidMetrics(pmce, promTs, buf, &pidMetricsCtx.pmStats.GeneratedCount)
			delete(pmc, pidTid)
			dCnt += 1
			if buf.Len() >= BUF_MAX_SIZE {
				bytesCount += buf.Len()
				if wChan != nil {
					wChan <- buf
				} else {
					bufPool.ReturnBuffer(buf)
				}
				buf = bufPool.GetBuffer()
			}
		} else {
			pmcCnt += 1
		}
	}
	// Flush the last buffer:
	bytesCount += buf.Len()
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

	// Update stats:
	pidMetricsCtx.pmStats.DeletedCount += dCnt
	pidMetricsCtx.pmStats.PmcCount = pmcCnt
	pidMetricsCtx.pmStats.BytesCount = uint64(bytesCount)

	// Publish stats:
	if pidMetricsCtx.enableStatsMetrics && wChan != nil {
		buf = bufPool.GetBuffer()
		pmStats := pidMetricsCtx.pmStats
		statsLabels := fmt.Sprintf(
			`%s="%s",%s="%s",%s="%d"`,
			HOSTNAME_LABEL_NAME, pidMetricsCtx.hostname,
			JOB_LABEL_NAME, pidMetricsCtx.job,
			PROC_PID_METRICS_STATS_PID_LIST_PART_LABEL_NAME, pidMetricsCtx.pidListPart,
		)
		fmt.Fprintf(
			buf,
			`%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s
%s{%s} %d %s

`,
			PROC_PID_METRICS_STATS_PID_COUNT_METRICS_NAME, statsLabels, pmStats.PidCount, promTs,
			PROC_PID_METRICS_STATS_TID_COUNT_METRICS_NAME, statsLabels, pmStats.TidCount, promTs,
			PROC_PID_METRICS_STATS_ACTIVE_COUNT_METRICS_NAME, statsLabels, pmStats.ActiveCount, promTs,
			PROC_PID_METRICS_STATS_FULL_METRICS_COUNT_METRICS_NAME, statsLabels, pmStats.FullMetricsCount, promTs,
			PROC_PID_METRICS_STATS_CREATED_COUNT_METRICS_NAME, statsLabels, pmStats.CreatedCount, promTs,
			PROC_PID_METRICS_STATS_DELETED_COUNT_METRICS_NAME, statsLabels, pmStats.DeletedCount, promTs,
			PROC_PID_METRICS_STATS_ERROR_COUNT_METRICS_NAME, statsLabels, pmStats.ErrorCount, promTs,
			PROC_PID_METRICS_STATS_GENERATED_COUNT_METRICS_NAME, statsLabels, pmStats.GeneratedCount, promTs,
			PROC_PID_METRICS_STATS_CACHE_COUNT_METRICS_NAME, statsLabels, pmStats.PmcCount, promTs,
			PROC_PID_METRICS_STATS_BYTES_COUNT_METRICS_NAME, statsLabels, pmStats.BytesCount, promTs,
		)
		wChan <- buf
	}
}
