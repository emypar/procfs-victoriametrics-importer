#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import dataclasses
from typing import List, Literal, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot
from tools_common import StructBase

from . import (
    proc_pid_cgroup_metrics,
    proc_pid_cmdline_metrics,
    proc_pid_io_metrics,
    proc_pid_stat_metrics,
    proc_pid_status_metrics,
)
from .common import Metric, get_metrics_fn_list

PmceStatFieldName = Literal[
    "ProcStat", "ProcStatus", "ProcIo", "_procCgroups", "_procCmdline"
]


@dataclasses.dataclass
class PidMetricsCacheEntry(StructBase):
    # The pass# when this entry was updated, needed for detecting out-of-scope
    # PIDs:
    PassNum: int = 0
    # Full metrics cycle:
    FullMetricsCycleNum: int = 0
    # Cache parsed data from /proc:
    ProcStat: Optional[procfs.ProcStat] = None
    ProcStatus: Optional[procfs.ProcStatus] = None
    ProcIo: Optional[procfs.ProcIO] = None
    # Cache raw data from /proc, for info that is expensive to parse and which
    # doesn't change often:
    RawCgroup: bytes = b""
    RawCmdline: bytes = b""
    # Stats acquisition time stamp (Prometheus style, milliseconds since the
    # epoch):
    Timestamp: int = 0
    # Cache common labels:
    CommonLabels: str = ""
    # Pseudo-categorical metrics; useful to cache them since most likely they
    # do not change from one pass to the next:
    ProcPidCgroupMetrics: set = dataclasses.field(default_factory=set)
    ProcPidCmdlineMetric: str = ""
    ProcPidStatStateMetric: str = ""
    ProcPidStatInfoMetric: str = ""
    ProcPidStatusInfoMetric: str = ""

    # Additional private members:
    _procCgroups: Optional[procfs.ProcPidCgroups] = None
    _procCmdline: Optional[procfs.ProcPidCmdline] = None


pmce_field_to_metrics_fn_map = {
    "ProcStat": proc_pid_stat_metrics.metrics_fn_map,
    "ProcStatus": proc_pid_status_metrics.metrics_fn_map,
    "ProcIo": proc_pid_io_metrics.metrics_fn_map,
    "_procCgroups": proc_pid_cgroup_metrics.metrics_fn_map,
    "_procCmdline": proc_pid_cmdline_metrics.metrics_fn_map,
}


def load_pmce(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
) -> PidMetricsCacheEntry:
    procStat = procfs.load_proc_pid_stat(pid, tid=tid, procfs_root=procfs_root)
    procStatus = procfs.load_proc_pid_status(pid, tid=tid, procfs_root=procfs_root)
    procIo = procfs.load_proc_pid_io(pid, tid=tid, procfs_root=procfs_root)
    procCgroups = procfs.load_proc_pid_cgroups(pid, tid=tid, procfs_root=procfs_root)
    procCmdline = procfs.load_proc_pid_cmdline(pid, tid=tid, procfs_root=procfs_root)
    common_labels = proc_pid_stat_metrics.make_common_labels(
        pid, tid=tid, procfs_root=procfs_root
    )
    ts = max(procStat._ts, procStatus._ts, procIo._ts, procCgroups._ts, procCmdline._ts)
    pmce = PidMetricsCacheEntry(
        ProcStat=procStat,
        ProcStatus=procStatus,
        ProcIo=procIo,
        RawCgroup=procCgroups.raw,
        RawCmdline=procCmdline.raw,
        Timestamp=ts,
        CommonLabels=common_labels,
    )
    update_pmce(pmce)
    return pmce


def update_pmce(
    pmce: PidMetricsCacheEntry,
    procStat: Optional[procfs.ProcStat] = None,
    procStatus: Optional[procfs.ProcStatus] = None,
    procIo: Optional[procfs.ProcIO] = None,
    rawCgroup: Optional[bytes] = None,
    rawCmdline: Optional[bytes] = None,
):
    if procStat is not None:
        pmce.ProcStat = procStat
    if pmce.ProcStat is not None:
        pmce.ProcPidStatStateMetric = proc_pid_stat_metrics.proc_pid_stat_state_metric(
            pmce.ProcStat, pmce.CommonLabels
        ).metric
        pmce.ProcPidStatInfoMetric = proc_pid_stat_metrics.proc_pid_stat_info_metric(
            pmce.ProcStat, pmce.CommonLabels
        ).metric
    else:
        pmce.ProcPidStatStateMetric = ""
        pmce.ProcPidStatInfoMetric = ""
    if procStatus is not None:
        pmce.ProcStatus = procStatus
    if pmce.ProcStatus is not None:
        pmce.ProcPidStatusInfoMetric = (
            proc_pid_status_metrics.proc_pid_status_info_metric(
                pmce.ProcStatus, pmce.CommonLabels
            ).metric
        )
    else:
        pmce.ProcPidStatusInfoMetric = ""
    if procIo is not None:
        pmce.ProcIo = procIo
    if rawCgroup is not None:
        pmce.RawCgroup = rawCgroup
    if pmce.RawCgroup is not None:
        pmce._procCgroups = procfs.proc_cgroup.ProcPidCgroups(pmce.RawCgroup)
        pmce.ProcPidCgroupMetrics = set(
            m.metric
            for m in proc_pid_cgroup_metrics.proc_pid_cgroup_metrics(
                pmce._procCgroups, pmce.CommonLabels
            )
        )
    else:
        pmce.ProcPidCgroupMetrics = set()
    if rawCmdline is not None:
        pmce.RawCmdline = rawCmdline
    if pmce.RawCmdline is not None:
        pmce._procCmdline = procfs.proc_cmdline.ProcPidCmdline(pmce.RawCmdline)
        pmce.ProcPidCmdlineMetric = proc_pid_cmdline_metrics.proc_pid_cmdline_metrics(
            pmce._procCmdline, pmce.CommonLabels
        ).metric
    else:
        pmce.ProcPidCmdlineMetric = ""


def generate_pmce_full_metrics(
    pmce: PidMetricsCacheEntry,
    ts: Optional[int] = None,
    prev_pmce: Optional[PidMetricsCacheEntry] = None,
    prev_ts: Optional[int] = None,
) -> List[Metric]:
    if ts is None:
        ts = pmce.Timestamp
    metrics = []

    for pmce_stat_field, metrics_fn_map in pmce_field_to_metrics_fn_map.items():
        for metric_fn, require_history in get_metrics_fn_list(metrics_fn_map):
            metric_or_metrics = None
            if require_history:
                if prev_pmce is not None:
                    metric_or_metrics = metric_fn(
                        getattr(pmce, pmce_stat_field),
                        getattr(prev_pmce, pmce_stat_field),
                        pmce.CommonLabels,
                        ts=ts,
                        prev_ts=prev_ts,
                    )
            else:
                metric_or_metrics = metric_fn(
                    getattr(pmce, pmce_stat_field), pmce.CommonLabels, ts=ts
                )
            if metric_or_metrics is not None:
                if isinstance(metric_or_metrics, list):
                    metrics.extend(metric_or_metrics)
                else:
                    metrics.append(metric_or_metrics)

    return metrics


def generate_pcme_pseudo_categorical_change_metrics(
    pmce: PidMetricsCacheEntry,
    prev_pmce: PidMetricsCacheEntry,
    ts: Optional[int] = None,
    clear_only: bool = False,
) -> List[Metric]:
    if ts is None:
        ts = pmce.Timestamp

    prev_metrics_set = prev_pmce.ProcPidCgroupMetrics | set(
        [
            prev_pmce.ProcPidCmdlineMetric,
            prev_pmce.ProcPidStatStateMetric,
            prev_pmce.ProcPidStatInfoMetric,
            prev_pmce.ProcPidStatusInfoMetric,
        ]
    )

    crt_metrics_set = pmce.ProcPidCgroupMetrics | set(
        [
            pmce.ProcPidCmdlineMetric,
            pmce.ProcPidStatStateMetric,
            pmce.ProcPidStatInfoMetric,
            pmce.ProcPidStatusInfoMetric,
        ]
    )

    metrics = []
    for metric in prev_metrics_set - crt_metrics_set:
        metrics.append(Metric(metric=metric, val=0, ts=ts))
    if not clear_only:
        for metric in crt_metrics_set - prev_metrics_set:
            metrics.append(Metric(metric=metric, val=1, ts=ts))
    return metrics


def generate_pcme_pseudo_categorical_clear_metrics(
    pmce: PidMetricsCacheEntry,
    ts: Optional[int] = None,
) -> List[Metric]:
    if ts is None:
        ts = pmce.Timestamp
    metrics = []
    for metric in pmce.ProcPidCgroupMetrics:
        metrics.append(Metric(metric=metric, val=0, ts=ts))
    metrics.append(Metric(metric=pmce.ProcPidCmdlineMetric, val=0, ts=ts))
    metrics.append(Metric(metric=pmce.ProcPidStatStateMetric, val=0, ts=ts))
    metrics.append(Metric(metric=pmce.ProcPidStatInfoMetric, val=0, ts=ts))
    metrics.append(Metric(metric=pmce.ProcPidStatusInfoMetric, val=0, ts=ts))
    return metrics
