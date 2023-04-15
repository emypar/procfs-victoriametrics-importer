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
	"os"
	"path/filepath"
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
}

// The context for pid metrics generation:
type PidMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration
	// Procfs dir root, needed to read raw data:
	procfsRoot string
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
	// Full metrics cycle seed, using to initialize the PID full metrics cycle
	// counter at PID cache entry creation time. It will be changed after each
	// use.
	fullMetricsCycleSeed int
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
	// The following are useful for testing, in lieu of mocks:
	hostname  string
	job       string
	clktckSec float64
	timeNow   TimeNowFn
	getPids   func(int) []PidTidPair
	bufPool   *BufferPool
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
		interval:          interval,
		procfsRoot:        procfsRoot,
		fs:                fs,
		pidListPart:       pidListPart,
		pmc:               PidMetricsCache{},
		psc:               PidStarttimeCache{},
		fullMetricsFactor: fullMetricsFactor,
		activeThreshold:   activeThreshold,
		passNum:           0,
		wChan:             wChan,
		hostname:          hostname,
		job:               job,
		clktckSec:         clktckSec,
		timeNow:           timeNow,
		getPids:           getPids,
		bufPool:           bufPool,
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

func (pidMetricsCtx *PidMetricsContext) GetFullMetricsCycleSeed() int {
	pidMetricsCtx.fullMetricsCycleSeed += 1
	if pidMetricsCtx.fullMetricsCycleSeed > pidMetricsCtx.fullMetricsFactor {
		pidMetricsCtx.fullMetricsCycleSeed = 1
	}
	return pidMetricsCtx.fullMetricsCycleSeed
}

// Previous pass cache, required for pseudo-categorical metrics and for %CPU.
type PidMetricsCacheEntry struct {
	// The pass# when this entry was updated, needed for detecting out-of-scope
	// PIDs:
	PassNum uint64
	// Full metrics cycle:
	FullMetricsCycleNum int
	// Cache parsed data from /proc:
	ProcStat   *procfs.ProcStat
	ProcStatus *procfs.ProcStatus
	ProcIo     *procfs.ProcIO
	// Cache raw data from /proc, for info that is expensive to parse and which
	// doesn't change often:
	RawCgroup  []byte
	RawCmdline []byte
	// Stats acquisition time stamp (Prometheus style, milliseconds since the
	// epoch):
	Timestamp int64
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
	if pmce != nil {
		starttime = pmce.ProcStat.Starttime
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
	var proc procfs.Proc
	var procStat *procfs.ProcStat
	var procStatus *procfs.ProcStatus
	var procIo *procfs.ProcIO
	var rawCgroup []byte
	var rawCmdline []byte
	var err error
	var procDir string
	var generatedCount uint64 = 0

	pmc := pidMetricsCtx.pmc
	psc := pidMetricsCtx.psc
	pmStats := pidMetricsCtx.pmStats

	pid, tid := pidTid.pid, pidTid.tid
	isForPid := tid == 0
	isActive, fullMetrics, prevDeleted := false, pidMetricsCtx.fullMetricsFactor <= 1, false

	// Cache entry: new v. update:
	pmce, exists := pmc[pidTid]
	bufInitLen := buf.Len()

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
			if !exists && pmce != nil {
				pmc[pidTid] = pmce
			}
		} else {
			if buf.Len() > bufInitLen {
				buf.Truncate(bufInitLen)
			}
		}
	}()

	if isForPid {
		proc, err = pidMetricsCtx.fs.Proc(pid)
		procDir = filepath.Join(pidMetricsCtx.procfsRoot, strconv.Itoa(pid))
	} else {
		proc, err = pidMetricsCtx.fs.Thread(pid, tid)
		procDir = filepath.Join(pidMetricsCtx.procfsRoot, strconv.Itoa(pid), "task", strconv.Itoa(tid))
	}
	if err != nil {
		return err
	}
	procStatRet, err := proc.Stat()
	if err != nil {
		return err
	}
	procStat = &procStatRet
	if exists && pmce.ProcStat.Starttime != procStat.Starttime {
		// A different instance of the same PID/TID, clear the previous one's
		// pseudo-categorical metrics:
		clearProcPidMetrics(
			pmce,
			strconv.FormatInt(pidMetricsCtx.timeNow().UnixMilli(), 10),
			buf,
			&generatedCount,
		)
		// Readjust the buffer recovery mark to accommodate the above:
		bufInitLen = buf.Len()

		delete(pmc, pidTid)
		pmce, exists, prevDeleted = nil, false, true
	}

	if exists {
		if pidMetricsCtx.activeThreshold == 0 ||
			(procStat.UTime+procStat.STime)-(pmce.ProcStat.UTime+pmce.ProcStat.STime) >= pidMetricsCtx.activeThreshold {
			isActive = true
		}
		if pidMetricsCtx.fullMetricsFactor > 1 {
			pmce.FullMetricsCycleNum += 1
			if pmce.FullMetricsCycleNum > pidMetricsCtx.fullMetricsFactor {
				pmce.FullMetricsCycleNum = 1
			}
			if pmce.FullMetricsCycleNum == pidMetricsCtx.fullMetricsFactor {
				fullMetrics = true
			}
		}
	} else {
		fullMetrics = true
	}

	if isActive || fullMetrics { // || !exists {
		procStatusRet, err := proc.NewStatus()
		if err != nil {
			return err
		}
		procStatus = &procStatusRet
		procIoRet, err := proc.IO()
		if err != nil {
			return err
		}
		procIo = &procIoRet
	}
	if fullMetrics { // || !exists {
		rawCgroup, err = os.ReadFile(filepath.Join(procDir, "cgroup"))
		if err != nil {
			return err
		}
		rawCmdline, err = os.ReadFile(filepath.Join(procDir, "cmdline"))
		if err != nil {
			return err
		}
	}

	// Prometheus timestamp: milliseconds since epoch:
	timestamp := pidMetricsCtx.timeNow().UnixMilli()
	// As string, for metrics generation:
	metricTs := strconv.FormatInt(timestamp, 10)

	// Keep track of changed pseudo-categorical metrics, needed to generate
	// metrics on updates only.
	cgroupChanged := false
	cmdlineChanged := false
	statStateChanged := false
	statInfoChanged := false
	statusInfoChanged := false

	if !exists {
		pmce = &PidMetricsCacheEntry{
			RawCgroup:  rawCgroup,
			RawCmdline: rawCmdline,
		}
		if pidMetricsCtx.fullMetricsFactor > 1 {
			pmce.FullMetricsCycleNum = pidMetricsCtx.GetFullMetricsCycleSeed()
		}
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
			rawCgroup,
			metricTs,
			buf,
			&generatedCount,
		)
		if err != nil {
			return err
		}
		updateProcPidCmdlineMetric(pmce, rawCmdline, metricTs, buf, &generatedCount)
		updateProcPidStatStateMetric(pmce, procStat, metricTs, buf, &generatedCount)
		updateProcPidStatInfoMetric(pmce, procStat, metricTs, buf, &generatedCount)
		updateProcPidStatusInfoMetric(pmce, procStatus, metricTs, buf, &generatedCount)
	} else {
		// Check for changes in pseudo-categorical metrics:
		pmceStat := pmce.ProcStat
		if pmceStat.State != procStat.State {
			updateProcPidStatStateMetric(pmce, procStat, metricTs, buf, &generatedCount)
			statStateChanged = true
		}
		if pmceStat.Comm != procStat.Comm ||
			pmceStat.PPID != procStat.PPID ||
			pmceStat.PGRP != procStat.PGRP ||
			pmceStat.Session != procStat.Session ||
			pmceStat.TTY != procStat.TTY ||
			pmceStat.TPGID != procStat.TPGID ||
			pmceStat.Flags != procStat.Flags ||
			pmceStat.Priority != procStat.Priority ||
			pmceStat.Nice != procStat.Nice ||
			pmceStat.RTPriority != procStat.RTPriority ||
			pmceStat.Policy != procStat.Policy {
			updateProcPidStatInfoMetric(pmce, procStat, metricTs, buf, &generatedCount)
			statInfoChanged = true
		}

		if procStatus != nil {
			pmceStatus := pmce.ProcStatus
			if pmceStatus.Name != procStatus.Name ||
				pmceStatus.TGID != procStatus.TGID ||
				pmceStatus.UIDs != procStatus.UIDs ||
				pmceStatus.GIDs != procStatus.GIDs {
				updateProcPidStatusInfoMetric(pmce, procStatus, metricTs, buf, &generatedCount)
				statusInfoChanged = true
			}
		}

		if rawCgroup != nil && !bytes.Equal(pmce.RawCgroup, rawCgroup) {
			err = updateProcPidCgroupMetric(pmce, rawCgroup, metricTs, buf, &generatedCount)
			if err != nil {
				return err
			}
			pmce.RawCgroup = rawCgroup
			cgroupChanged = true
		}

		if rawCmdline != nil && !bytes.Equal(pmce.RawCmdline, rawCmdline) {
			updateProcPidCmdlineMetric(pmce, rawCmdline, metricTs, buf, &generatedCount)
			pmce.RawCmdline = rawCmdline
			cmdlineChanged = true
		}
	}

	// Generate pseudo-categorical metrics; they all share the same value
	// timestamp suffix:
	setSuffix := []byte(" 1 " + metricTs + "\n")

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

	// procStat is always available:

	// proc_pid_stat_minflt:
	if fullMetrics || procStat.MinFlt != pmce.ProcStat.MinFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_MINFLT_METRIC_NAME,
			commonLabels,
			procStat.MinFlt,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cminflt:
	if fullMetrics || procStat.CMinFlt != pmce.ProcStat.CMinFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CMINFLT_METRIC_NAME,
			commonLabels,
			procStat.CMinFlt,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_majflt:
	if fullMetrics || procStat.MajFlt != pmce.ProcStat.MajFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_MAJFLT_METRIC_NAME,
			commonLabels,
			procStat.MajFlt,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cmajflt:
	if fullMetrics || procStat.CMajFlt != pmce.ProcStat.CMajFlt {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CMAJFLT_METRIC_NAME,
			commonLabels,
			procStat.CMajFlt,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_utime_seconds:
	pCpuChanged := false
	if fullMetrics || procStat.UTime != pmce.ProcStat.UTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_UTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.UTime)*pidMetricsCtx.clktckSec,
			metricTs,
		)
		pCpuChanged = true
		generatedCount += 1
	}

	// proc_pid_stat_stime_seconds:
	if fullMetrics || procStat.STime != pmce.ProcStat.STime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_STIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.STime)*pidMetricsCtx.clktckSec,
			metricTs,
		)
		pCpuChanged = true
		generatedCount += 1
	}

	// proc_pid_stat_cutime_seconds:
	if fullMetrics || procStat.CUTime != pmce.ProcStat.CUTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_CUTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.CUTime)*pidMetricsCtx.clktckSec,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cstime_seconds:
	if fullMetrics || procStat.CSTime != pmce.ProcStat.CSTime {
		fmt.Fprintf(
			buf,
			"%s{%s} %.06f %s\n",
			PROC_PID_STAT_CSTIME_SECONDS_METRIC_NAME,
			commonLabels,
			float64(procStat.CSTime)*pidMetricsCtx.clktckSec,
			metricTs,
		)
		generatedCount += 1
	}

	// %CPU, but only if there was a previous pass:
	if exists && (fullMetrics || pCpuChanged) {
		// %CPU = (procStat.XTime - pmce.ProcStat.XTime) * pidMetricsCtx.clktckSec / ((timestamp - pmce.timestamp)/1000) * 100
		dUTime := procStat.UTime - pmce.ProcStat.UTime
		dSTime := procStat.STime - pmce.ProcStat.STime
		dTimeSec := float64(timestamp-pmce.Timestamp) / 1000. // Prometheus millisec -> sec
		pCpuFactor := pidMetricsCtx.clktckSec / dTimeSec * 100.
		fmt.Fprintf(
			buf,
			"%s{%s} %.02f %s\n",
			PROC_PID_STAT_UTIME_PCT_METRIC_NAME,
			commonLabels,
			float64(dUTime)*pCpuFactor,
			metricTs,
		)
		fmt.Fprintf(
			buf,
			"%s{%s} %.02f %s\n",
			PROC_PID_STAT_STIME_PCT_METRIC_NAME,
			commonLabels,
			float64(dSTime)*pCpuFactor,
			metricTs,
		)
		fmt.Fprintf(
			buf,
			"%s{%s} %.02f %s\n",
			PROC_PID_STAT_CPU_TIME_PCT_METRIC_NAME,
			commonLabels,
			float64(dUTime+dSTime)*pCpuFactor,
			metricTs,
		)
		generatedCount += 2
	}

	// proc_pid_stat_vsize:
	if fullMetrics || procStat.VSize != pmce.ProcStat.VSize {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_VSIZE_METRIC_NAME,
			commonLabels,
			procStat.VSize,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_rss:
	if fullMetrics || procStat.RSS != pmce.ProcStat.RSS {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_RSS_METRIC_NAME,
			commonLabels,
			procStat.RSS,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_rsslim:
	if fullMetrics || procStat.RSSLimit != pmce.ProcStat.RSSLimit {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_RSSLIM_METRIC_NAME,
			commonLabels,
			procStat.RSSLimit,
			metricTs,
		)
		generatedCount += 1
	}

	// proc_pid_stat_cpu:
	if fullMetrics || procStat.Processor != pmce.ProcStat.Processor {
		fmt.Fprintf(
			buf,
			"%s{%s} %d %s\n",
			PROC_PID_STAT_CPU_METRIC_NAME,
			commonLabels,
			procStat.Processor,
			metricTs,
		)
		generatedCount += 1
	}
	pmce.ProcStat = procStat

	if procStatus != nil {
		// proc_pid_status_vm_peak:
		if fullMetrics || procStatus.VmPeak != pmce.ProcStatus.VmPeak {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_PEAK_METRIC_NAME,
				commonLabels,
				procStatus.VmPeak,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_size:
		if fullMetrics || procStatus.VmSize != pmce.ProcStatus.VmSize {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_SIZE_METRIC_NAME,
				commonLabels,
				procStatus.VmSize,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_lck:
		if fullMetrics || procStatus.VmLck != pmce.ProcStatus.VmLck {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_LCK_METRIC_NAME,
				commonLabels,
				procStatus.VmLck,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_pin:
		if fullMetrics || procStatus.VmPin != pmce.ProcStatus.VmPin {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_PIN_METRIC_NAME,
				commonLabels,
				procStatus.VmPin,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_hwm:
		if fullMetrics || procStatus.VmHWM != pmce.ProcStatus.VmHWM {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_HWM_METRIC_NAME,
				commonLabels,
				procStatus.VmHWM,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_rss:
		if fullMetrics || procStatus.VmRSS != pmce.ProcStatus.VmRSS {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_RSS_METRIC_NAME,
				commonLabels,
				procStatus.VmRSS,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_rss_anon:
		if fullMetrics || procStatus.RssAnon != pmce.ProcStatus.RssAnon {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_RSS_ANON_METRIC_NAME,
				commonLabels,
				procStatus.RssAnon,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_rss_file:
		if fullMetrics || procStatus.RssFile != pmce.ProcStatus.RssFile {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_RSS_FILE_METRIC_NAME,
				commonLabels,
				procStatus.RssFile,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_rss_shmem:
		if fullMetrics || procStatus.RssShmem != pmce.ProcStatus.RssShmem {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_RSS_SHMEM_METRIC_NAME,
				commonLabels,
				procStatus.RssShmem,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_data:
		if fullMetrics || procStatus.VmData != pmce.ProcStatus.VmData {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_DATA_METRIC_NAME,
				commonLabels,
				procStatus.VmData,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_stk:
		if fullMetrics || procStatus.VmStk != pmce.ProcStatus.VmStk {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_STK_METRIC_NAME,
				commonLabels,
				procStatus.VmStk,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_exe:
		if fullMetrics || procStatus.VmExe != pmce.ProcStatus.VmExe {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_EXE_METRIC_NAME,
				commonLabels,
				procStatus.VmExe,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_lib:
		if fullMetrics || procStatus.VmLib != pmce.ProcStatus.VmLib {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_LIB_METRIC_NAME,
				commonLabels,
				procStatus.VmLib,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_pte:
		if fullMetrics || procStatus.VmPTE != pmce.ProcStatus.VmPTE {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_PTE_METRIC_NAME,
				commonLabels,
				procStatus.VmPTE,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_pmd:
		if fullMetrics || procStatus.VmPMD != pmce.ProcStatus.VmPMD {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_PMD_METRIC_NAME,
				commonLabels,
				procStatus.VmPMD,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_vm_swap:
		if fullMetrics || procStatus.VmSwap != pmce.ProcStatus.VmSwap {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VM_SWAP_METRIC_NAME,
				commonLabels,
				procStatus.VmSwap,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_hugetbl_pages:
		if fullMetrics || procStatus.HugetlbPages != pmce.ProcStatus.HugetlbPages {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_HUGETBL_PAGES_METRIC_NAME,
				commonLabels,
				procStatus.HugetlbPages,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_voluntary_ctxt_switches:
		if fullMetrics || procStatus.VoluntaryCtxtSwitches != pmce.ProcStatus.VoluntaryCtxtSwitches {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_VOLUNTARY_CTXT_SWITCHES_METRIC_NAME,
				commonLabels,
				procStatus.VoluntaryCtxtSwitches,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_status_nonvoluntary_ctxt_switches:
		if fullMetrics || procStatus.NonVoluntaryCtxtSwitches != pmce.ProcStatus.NonVoluntaryCtxtSwitches {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_STATUS_NONVOLUNTARY_CTXT_SWITCHES_METRIC_NAME,
				commonLabels,
				procStatus.NonVoluntaryCtxtSwitches,
				metricTs,
			)
			generatedCount += 1
		}

		pmce.ProcStatus = procStatus
	}

	if procIo != nil {
		// proc_pid_io_rcar:
		if fullMetrics || procIo.RChar != pmce.ProcIo.RChar {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_RCAR_METRIC_NAME,
				commonLabels,
				procIo.RChar,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_wcar:
		if fullMetrics || procIo.WChar != pmce.ProcIo.WChar {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_WCAR_METRIC_NAME,
				commonLabels,
				procIo.WChar,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_syscr:
		if fullMetrics || procIo.SyscR != pmce.ProcIo.SyscR {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_SYSCR_METRIC_NAME,
				commonLabels,
				procIo.SyscR,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_syscw:
		if fullMetrics || procIo.SyscW != pmce.ProcIo.SyscW {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_SYSCW_METRIC_NAME,
				commonLabels,
				procIo.SyscW,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_readbytes:
		if fullMetrics || procIo.ReadBytes != pmce.ProcIo.ReadBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_READBYTES_METRIC_NAME,
				commonLabels,
				procIo.ReadBytes,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_writebytes:
		if fullMetrics || procIo.WriteBytes != pmce.ProcIo.WriteBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_WRITEBYTES_METRIC_NAME,
				commonLabels,
				procIo.WriteBytes,
				metricTs,
			)
			generatedCount += 1
		}

		// proc_pid_io_cancelled_writebytes:
		if fullMetrics || procIo.CancelledWriteBytes != pmce.ProcIo.CancelledWriteBytes {
			fmt.Fprintf(
				buf,
				"%s{%s} %d %s\n",
				PROC_PID_IO_CANCELLED_WRITEBYTES_METRIC_NAME,
				commonLabels,
				procIo.CancelledWriteBytes,
				metricTs,
			)
			generatedCount += 1
		}

		pmce.ProcIo = procIo
	}

	// Update the cache entry:
	pmce.PassNum = pidMetricsCtx.passNum
	pmce.Timestamp = timestamp

	return nil
}

// Clear pseudo-categorical metrics for out-of-scope PID:
func clearProcPidMetrics(
	pmce *PidMetricsCacheEntry,
	metricTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	gCnt := uint64(0)
	clearSuffix := []byte(" 0 " + metricTs + "\n")
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
	for _, pidTid := range pidList {
		err := GeneratePidMetrics(pidTid, pidMetricsCtx, buf)
		if err != nil {
			PidMetricsLog.Warnf("GeneratePidMetrics(%v): %s", pidTid, err)
		}
		if buf.Len() >= BUF_MAX_SIZE {
			if wChan != nil {
				wChan <- buf
			} else {
				bufPool.ReturnBuffer(buf)
			}
			buf = bufPool.GetBuffer()
		}
	}

	// Clear expired PIDs:
	metricTs := strconv.FormatInt(timestamp, 10)
	dCnt, pmcCnt := uint64(0), uint64(0)
	pmc := pidMetricsCtx.pmc
	for pidTid, pmce := range pmc {
		if pmce.PassNum != passNum {
			clearProcPidMetrics(pmce, metricTs, buf, &pidMetricsCtx.pmStats.GeneratedCount)
			delete(pmc, pidTid)
			dCnt += 1
			if buf.Len() >= BUF_MAX_SIZE {
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
	pidMetricsCtx.pmStats.DeletedCount += dCnt
	pidMetricsCtx.pmStats.PmcCount = pmcCnt
	// Flush the last buffer:
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}
}
