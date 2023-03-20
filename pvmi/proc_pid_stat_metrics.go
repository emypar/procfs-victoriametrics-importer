// proc_pid_stat_* metrics utils.

package pvmi

import (
	"bytes"
	"fmt"

	"github.com/prometheus/procfs"
)

const (
	// proc_pid_stat_state:
	PROC_PID_STAT_STATE_METRIC_NAME             = "proc_pid_stat_state"
	PROC_PID_STAT_STATE_METRIC_STATE_LABEL_NAME = "state"

	// proc_pid_stat_info:
	PROC_PID_STAT_INFO_METRIC_NAME                   = "proc_pid_stat_info"
	PROC_PID_STAT_INFO_METRIC_COMM_LABEL_NAME        = "comm"
	PROC_PID_STAT_INFO_METRIC_PPID_LABEL_NAME        = "ppid"
	PROC_PID_STAT_INFO_METRIC_PGRP_LABEL_NAME        = "pgrp"
	PROC_PID_STAT_INFO_METRIC_SESSION_LABEL_NAME     = "session"
	PROC_PID_STAT_INFO_METRIC_TTY_NR_LABEL_NAME      = "tty_nr"
	PROC_PID_STAT_INFO_METRIC_TPGID_LABEL_NAME       = "tpgid"
	PROC_PID_STAT_INFO_METRIC_FLAGS_LABEL_NAME       = "flags"
	PROC_PID_STAT_INFO_METRIC_PRIORITY_LABEL_NAME    = "priority"
	PROC_PID_STAT_INFO_METRIC_NICE_LABEL_NAME        = "nice"
	PROC_PID_STAT_INFO_METRIC_RT_PRIORITY_LABEL_NAME = "rt_priority"
	PROC_PID_STAT_INFO_METRIC_POLICY_LABEL_NAME      = "policy"
	PROC_PID_STAT_MINFLT_METRIC_NAME                 = "proc_pid_stat_minflt"

	PROC_PID_STAT_CMINFLT_METRIC_NAME = "proc_pid_stat_cminflt"

	PROC_PID_STAT_MAJFLT_METRIC_NAME = "proc_pid_stat_majflt"

	PROC_PID_STAT_CMAJFLT_METRIC_NAME = "proc_pid_stat_cmajflt"

	PROC_PID_STAT_UTIME_SECONDS_METRIC_NAME = "proc_pid_stat_utime_seconds"

	PROC_PID_STAT_STIME_SECONDS_METRIC_NAME = "proc_pid_stat_stime_seconds"

	PROC_PID_STAT_CUTIME_SECONDS_METRIC_NAME = "proc_pid_stat_cutime_seconds"

	PROC_PID_STAT_CSTIME_SECONDS_METRIC_NAME = "proc_pid_stat_cstime_seconds"

	PROC_PID_STAT_UTIME_PCT_METRIC_NAME = "proc_pid_stat_utime_pct"

	PROC_PID_STAT_STIME_PCT_METRIC_NAME = "proc_pid_stat_stime_pct"

	PROC_PID_STAT_CPU_TIME_PCT_METRIC_NAME = "proc_pid_stat_cpu_time_pct"

	PROC_PID_STAT_CCPU_TIME_PCT_METRIC_NAME = "proc_pid_stat_ccpu_time_pct"

	PROC_PID_STAT_VSIZE_METRIC_NAME = "proc_pid_stat_vsize"

	PROC_PID_STAT_RSS_METRIC_NAME = "proc_pid_stat_rss"

	PROC_PID_STAT_RSSLIM_METRIC_NAME = "proc_pid_stat_rsslim"

	PROC_PID_STAT_CPU_METRIC_NAME = "proc_pid_stat_cpu"
)

var pidStatStateMetricFmt string = fmt.Sprintf(
	`%s{%%s,%s="%%s"}`,
	PROC_PID_STAT_STATE_METRIC_NAME,
	PROC_PID_STAT_STATE_METRIC_STATE_LABEL_NAME,
)

var pidStatInfoMetricFmt string = fmt.Sprintf(
	`%s{%%s,%s="%%s",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d",%s="%%d"}`,
	PROC_PID_STAT_INFO_METRIC_NAME,
	PROC_PID_STAT_INFO_METRIC_COMM_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_PPID_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_PGRP_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_SESSION_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_TTY_NR_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_TPGID_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_FLAGS_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_PRIORITY_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_NICE_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_RT_PRIORITY_LABEL_NAME,
	PROC_PID_STAT_INFO_METRIC_POLICY_LABEL_NAME,
)

// Invoke for new/change in procStat state:
func updateProcPidStatStateMetric(
	pmce *PidMetricsCacheEntry,
	procStat *procfs.ProcStat,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	// Clear previous metric, if any:
	if pmce.ProcPidStatStateMetric != "" {
		buf.WriteString(pmce.ProcPidStatStateMetric + " 0 " + promTs + "\n")
		if generatedCount != nil {
			*generatedCount += 1
		}
	}
	// Update the metric:
	pmce.ProcPidStatStateMetric = fmt.Sprintf(
		pidStatStateMetricFmt,
		pmce.CommonLabels,
		procStat.State,
	)
	return nil
}

// Invoke for new/change in procStat info:
func updateProcPidStatInfoMetric(
	pmce *PidMetricsCacheEntry,
	procStat *procfs.ProcStat,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	// Clear previous metric, if any:
	if pmce.ProcPidStatInfoMetric != "" {
		buf.WriteString(pmce.ProcPidStatInfoMetric + " 0 " + promTs + "\n")
		if generatedCount != nil {
			*generatedCount += 1
		}
	}
	// Update the metric:
	pmce.ProcPidStatInfoMetric = fmt.Sprintf(
		pidStatInfoMetricFmt,
		pmce.CommonLabels,
		procStat.Comm,
		procStat.PPID,
		procStat.PGRP,
		procStat.Session,
		procStat.TTY,
		procStat.TPGID,
		procStat.Flags,
		procStat.Priority,
		procStat.Nice,
		procStat.RTPriority,
		procStat.Policy,
	)
	return nil
}
