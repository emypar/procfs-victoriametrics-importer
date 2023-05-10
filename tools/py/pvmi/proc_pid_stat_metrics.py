#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import functools
from typing import List, Optional

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot, TestHostname, TestJob

from .common import Metric, sanitize_label_value, ts_to_prometheus_ts


@functools.lru_cache(maxsize=None)
def make_common_labels(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> str:
    starttime = procfs.load_proc_pid_stat(
        pid, tid=tid, procfs_root=procfs_root
    ).Starttime
    if tid == 0:
        return (
            f'hostname="{_hostname}",job="{_job}",pid="{pid}",starttime="{starttime}"'
        )
    else:
        t_starttime = starttime
        if pid != tid:
            starttime = procfs.load_proc_pid_stat(
                pid, tid=0, procfs_root=procfs_root
            ).Starttime
        return (
            f'hostname="{_hostname}",job="{_job}",pid="{pid}",starttime="{starttime}"'
            f',tid="{tid}",t_starttime="{t_starttime}"'
        )


PROC_PID_STAT_STATE_METRIC_NAME = "proc_pid_stat_state"
PROC_PID_STAT_INFO_METRIC_NAME = "proc_pid_stat_info"

proc_pid_stat_categorical_metric_names = [
    PROC_PID_STAT_STATE_METRIC_NAME,
    PROC_PID_STAT_INFO_METRIC_NAME,
]


def generate_proc_pid_stat_metrics(
    proc_pid_stat: procfs.ProcStat,
    common_labels: str,
    ts: Optional[float] = None,
    prev_proc_pid_stat: Optional[procfs.ProcStat] = None,
    prev_ts: Optional[float] = None,
) -> List[Metric]:
    if ts is None:
        ts = proc_pid_stat._ts
    labels_sep = "," if common_labels else ""
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = [
        Metric(
            metric=(
                f'proc_pid_stat_state{{{common_labels}{labels_sep}state="{proc_pid_stat.State}"}}'
            ),
            val=1,
            ts=prom_ts,
        ),
        Metric(
            metric=(
                f"proc_pid_stat_info{{{common_labels}{labels_sep}"
                f'comm="{sanitize_label_value(proc_pid_stat.Comm)}"'
                f',ppid="{proc_pid_stat.PPID}"'
                f',pgrp="{proc_pid_stat.PGRP}"'
                f',session="{proc_pid_stat.Session}"'
                f',tty_nr="{proc_pid_stat.TTY}"'
                f',tpgid="{proc_pid_stat.TPGID}"'
                f',flags="{proc_pid_stat.Flags}"'
                f',priority="{proc_pid_stat.Priority}"'
                f',nice="{proc_pid_stat.Nice}"'
                f',rt_priority="{proc_pid_stat.RTPriority}"'
                f',policy="{proc_pid_stat.Policy}"'
                "}"
            ),
            val=1,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_minflt{{{common_labels}}}",
            val=proc_pid_stat.MinFlt,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_cminflt{{{common_labels}}}",
            val=proc_pid_stat.CMinFlt,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_majflt{{{common_labels}}}",
            val=proc_pid_stat.MajFlt,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_cmajflt{{{common_labels}}}",
            val=proc_pid_stat.CMajFlt,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_utime_seconds{{{common_labels}}}",
            val=proc_pid_stat.UTime * TestClktckSec,
            ts=prom_ts,
            valfmt=".06f",
        ),
        Metric(
            metric=f"proc_pid_stat_stime_seconds{{{common_labels}}}",
            val=proc_pid_stat.STime * TestClktckSec,
            ts=prom_ts,
            valfmt=".06f",
        ),
        Metric(
            metric=f"proc_pid_stat_cutime_seconds{{{common_labels}}}",
            val=proc_pid_stat.CUTime * TestClktckSec,
            ts=prom_ts,
            valfmt=".06f",
        ),
        Metric(
            metric=f"proc_pid_stat_cstime_seconds{{{common_labels}}}",
            val=proc_pid_stat.CSTime * TestClktckSec,
            ts=prom_ts,
            valfmt=".06f",
        ),
        Metric(
            metric=f"proc_pid_stat_vsize{{{common_labels}}}",
            val=proc_pid_stat.VSize,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_rss{{{common_labels}}}",
            val=proc_pid_stat.RSS,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_rsslim{{{common_labels}}}",
            val=proc_pid_stat.RSSLimit,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_stat_cpu{{{common_labels}}}",
            val=proc_pid_stat.Processor,
            ts=prom_ts,
        ),
    ]

    if prev_proc_pid_stat is not None:
        if prev_ts is None:
            prev_ts = prev_proc_pid_stat._ts
        delta_utime = proc_pid_stat.UTime - prev_proc_pid_stat.UTime
        delta_stime = proc_pid_stat.STime - prev_proc_pid_stat.STime
        delta_seconds = ts - prev_ts
        metrics.extend(
            [
                Metric(
                    metric=f"proc_pid_stat_utime_pct{{{common_labels}}}",
                    val=(delta_utime * TestClktckSec / delta_seconds) * 100.0,
                    ts=prom_ts,
                    valfmt=".02f",
                ),
                Metric(
                    metric=f"proc_pid_stat_stime_pct{{{common_labels}}}",
                    val=(delta_stime * TestClktckSec / delta_seconds) * 100.0,
                    ts=prom_ts,
                    valfmt=".02f",
                ),
                Metric(
                    metric=f"proc_pid_stat_cpu_time_pct{{{common_labels}}}",
                    val=(
                        ((delta_utime + delta_stime) * TestClktckSec / delta_seconds)
                        * 100.0
                    ),
                    ts=prom_ts,
                    valfmt=".02f",
                ),
            ]
        )

    return metrics
