#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import functools
from typing import List, Optional

import procfs
from metrics_common_test import (
    TestClktckSec,
    TestdataProcfsRoot,
    TestHostname,
    TestJob,
)
from tools_common import sanitize_label_value

from .common import Metric, register_metrics_fn


@functools.lru_cache(maxsize=None)
def make_common_labels(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    hostname: str = TestHostname,
    job: str = TestJob,
) -> str:
    starttime = procfs.load_proc_pid_stat(
        pid, tid=tid, procfs_root=procfs_root
    ).Starttime
    if tid == 0:
        return f'''hostname="{hostname}",job="{job}",pid="{pid}",starttime="{starttime}"'''
    else:
        t_starttime = starttime
        if pid != tid:
            starttime = procfs.load_proc_pid_stat(
                pid, tid=0, procfs_root=procfs_root
            ).Starttime
        return (
            f'''hostname="{hostname}",job="{job}",pid="{pid}",starttime="{starttime}"'''
            f''',tid="{tid}",t_starttime="{t_starttime}"'''
        )


metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_state_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
    _val: int = 1,
) -> Metric:
    return Metric(
        metric=(
            "proc_pid_stat_state{"
            + (
                common_labels
                + ("," if common_labels else "")
                + f'''state="{procStat.State}"'''
            )
            + "}"
        ),
        val=_val,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_info_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
    _val: int = 1,
) -> Metric:
    return Metric(
        metric=(
            "proc_pid_stat_info{"
            + (
                common_labels
                + ("," if common_labels else "")
                + f'''comm="{sanitize_label_value(procStat.Comm)}"'''
                + f''',ppid="{procStat.PPID}"'''
                + f''',pgrp="{procStat.PGRP}"'''
                + f''',session="{procStat.Session}"'''
                + f''',tty_nr="{procStat.TTY}"'''
                + f''',tpgid="{procStat.TPGID}"'''
                + f''',flags="{procStat.Flags}"'''
                + f''',priority="{procStat.Priority}"'''
                + f''',nice="{procStat.Nice}"'''
                + f''',rt_priority="{procStat.RTPriority}"'''
                + f''',policy="{procStat.Policy}"'''
            )
            + "}"
        ),
        val=_val,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_minflt_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_minflt" + "{" + common_labels + "}",
        val=procStat.MinFlt,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_cminflt_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_cminflt" + "{" + common_labels + "}",
        val=procStat.CMinFlt,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_majflt_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_majflt" + "{" + common_labels + "}",
        val=procStat.MajFlt,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_cmajflt_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_cmajflt" + "{" + common_labels + "}",
        val=procStat.CMajFlt,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_utime_seconds_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_utime_seconds" + "{" + common_labels + "}",
        val=procStat.UTime * TestClktckSec,
        ts=ts if ts is not None else procStat._ts,
        valfmt=".06f",
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_stime_seconds_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_stime_seconds" + "{" + common_labels + "}",
        val=procStat.STime * TestClktckSec,
        ts=ts if ts is not None else procStat._ts,
        valfmt=".06f",
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_cutime_seconds_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_cutime_seconds" + "{" + common_labels + "}",
        val=procStat.CUTime * TestClktckSec,
        ts=ts if ts is not None else procStat._ts,
        valfmt=".06f",
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_cstime_seconds_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_cstime_seconds" + "{" + common_labels + "}",
        val=procStat.CSTime * TestClktckSec,
        ts=ts if ts is not None else procStat._ts,
        valfmt=".06f",
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_vsize_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_vsize" + "{" + common_labels + "}",
        val=procStat.VSize,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_rss_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_rss" + "{" + common_labels + "}",
        val=procStat.RSS,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_rsslim_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_rsslim" + "{" + common_labels + "}",
        val=procStat.RSSLimit,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_pid_stat_cpu_metric(
    procStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_stat_cpu" + "{" + common_labels + "}",
        val=procStat.Processor,
        ts=ts if ts is not None else procStat._ts,
    )


@register_metrics_fn(metrics_fn_map, require_history=True)
def proc_pid_stat_pcpu_metrics(
    procStat: procfs.ProcStat,
    prevProcStat: procfs.ProcStat,
    common_labels: str = "",
    ts: Optional[int] = None,
    prev_ts: Optional[int] = None,
    _full_metrics: bool = False,
) -> List[Metric]:

    delta_utime = procStat.UTime - prevProcStat.UTime
    delta_stime = procStat.STime - prevProcStat.STime

    if not _full_metrics and delta_utime == 0 and delta_stime == 0:
        return []

    metrics = []
    if ts is None:
        ts = procStat._ts
    if prev_ts is None:
        prev_ts = prevProcStat._ts
    delta_seconds = (ts - prev_ts) / 1000.0  # Prom -> seconds
    metrics.append(
        Metric(
            metric="proc_pid_stat_utime_pct" + "{" + common_labels + "}",
            val=(delta_utime * TestClktckSec / delta_seconds) * 100.0,
            ts=ts,
            valfmt=".02f",
        )
    )
    metrics.append(
        Metric(
            metric="proc_pid_stat_stime_pct" + "{" + common_labels + "}",
            val=(delta_stime * TestClktckSec / delta_seconds) * 100.0,
            ts=ts,
            valfmt=".02f",
        )
    )
    metrics.append(
        Metric(
            metric="proc_pid_stat_cpu_time_pct" + "{" + common_labels + "}",
            val=(((delta_utime + delta_stime) * TestClktckSec / delta_seconds) * 100.0),
            ts=ts,
            valfmt=".02f",
        )
    )
    return metrics
