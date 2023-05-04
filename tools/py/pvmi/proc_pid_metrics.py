#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import dataclasses
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot
from tools_common import StructBase

from .common import Metric, prometheus_ts_to_ts, ts_to_prometheus_ts
from .proc_pid_cgroup_metrics import (
    generate_proc_pid_cgroup_metrics,
    proc_pid_cgroup_categorical_metric_names,
)
from .proc_pid_cmdline_metrics import (
    generate_proc_pid_cmdline_metrics,
    proc_pid_cmdline_categorical_metric_names,
)
from .proc_pid_io_metrics import generate_proc_pid_io_metrics
from .proc_pid_stat_metrics import (
    PROC_PID_STAT_INFO_METRIC_NAME,
    PROC_PID_STAT_STATE_METRIC_NAME,
    generate_proc_pid_stat_metrics,
    make_common_labels,
    proc_pid_stat_categorical_metric_names,
)
from .proc_pid_status_metrics import (
    PROC_PID_STATUS_INFO_METRIC_NAME,
    generate_proc_pid_status_metrics,
    proc_pid_status_categorical_metric_names,
)

proc_pid_categorical_metric_names = (
    proc_pid_cgroup_categorical_metric_names
    + proc_pid_cmdline_categorical_metric_names
    + proc_pid_stat_categorical_metric_names
    + proc_pid_status_categorical_metric_names
)


@dataclasses.dataclass
class PidMetricsCacheEntry(StructBase):
    # The pass# when this entry was updated, needed for detecting out-of-scope
    # PIDs:
    PassNum: int = 0
    # Refresh cycle#:
    RefreshCycleNum: int = 0
    # Cache parsed data from /proc:
    ProcStat: Optional[procfs.ProcStat] = None
    ProcStatus: Optional[procfs.ProcStatus] = None
    ProcIo: Optional[procfs.ProcIO] = None
    # Cache raw data from /proc, for info that is expensive to parse and which
    # doesn't change often:
    RawCgroup: bytes = b""
    RawCmdline: bytes = b""
    # Cache delta cpu time, needed for delta strategy for %CPU metrics; delta
    # cpu time is (STime - prevSTime)/(UTime - prevUTime). Since this is
    # available only from 2nd scan onwards, store it as a pointer that becomes
    # != nil only when this information is available.
    DeltaUTime: Optional[int] = None
    DeltaSTime: Optional[int] = None
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


PmceStatFieldNames = (
    "ProcStat",
    "ProcStatus",
    "ProcIo",
    "RawCgroup",
    "RawCmdline",
)


def load_pmce(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
) -> PidMetricsCacheEntry:
    proc_pid_stat = procfs.load_proc_pid_stat(pid, tid=tid, procfs_root=procfs_root)
    proc_pid_status = procfs.load_proc_pid_status(pid, tid=tid, procfs_root=procfs_root)
    proc_pid_io = procfs.load_proc_pid_io(pid, tid=tid, procfs_root=procfs_root)
    proc_pid_cgroups = procfs.load_proc_pid_cgroups(
        pid, tid=tid, procfs_root=procfs_root
    )
    proc_pid_cmdline = procfs.load_proc_pid_cmdline(
        pid, tid=tid, procfs_root=procfs_root
    )
    common_labels = make_common_labels(pid, tid=tid, procfs_root=procfs_root)
    ts = max(
        proc_pid_stat._ts,
        proc_pid_status._ts,
        proc_pid_io._ts,
        proc_pid_cgroups._ts,
        proc_pid_cmdline._ts,
    )
    pmce = PidMetricsCacheEntry(
        ProcStat=proc_pid_stat,
        ProcStatus=proc_pid_status,
        ProcIo=proc_pid_io,
        RawCgroup=proc_pid_cgroups.raw,
        RawCmdline=proc_pid_cmdline.raw,
        Timestamp=ts_to_prometheus_ts(ts),
        CommonLabels=common_labels,
    )
    update_pmce(pmce)
    return pmce


def update_pmce(
    pmce: PidMetricsCacheEntry,
    proc_pid_stat: Optional[procfs.ProcStat] = None,
    proc_pid_status: Optional[procfs.ProcStatus] = None,
    proc_pid_io: Optional[procfs.ProcIO] = None,
    rawCgroup: Optional[bytes] = None,
    rawCmdline: Optional[bytes] = None,
    prev_pmce: Optional[PidMetricsCacheEntry] = None,
):
    if proc_pid_stat is not None:
        pmce.ProcStat = proc_pid_stat
    if pmce.ProcStat is not None:
        metrics = generate_proc_pid_stat_metrics(pmce.ProcStat, pmce.CommonLabels)
        for m in metrics:
            m_name = m.name()
            if m_name == PROC_PID_STAT_STATE_METRIC_NAME:
                pmce.ProcPidStatStateMetric = m.metric
            elif m_name == PROC_PID_STAT_INFO_METRIC_NAME:
                pmce.ProcPidStatInfoMetric = m.metric
    else:
        pmce.ProcPidStatStateMetric = ""
        pmce.ProcPidStatInfoMetric = ""
    if proc_pid_status is not None:
        pmce.ProcStatus = proc_pid_status
    if pmce.ProcStatus is not None:
        metrics = generate_proc_pid_status_metrics(pmce.ProcStatus, pmce.CommonLabels)
        for m in metrics:
            m_name = m.name()
            if m_name == PROC_PID_STATUS_INFO_METRIC_NAME:
                pmce.ProcPidStatusInfoMetric = m.metric
    else:
        pmce.ProcPidStatusInfoMetric = ""
    if proc_pid_io is not None:
        pmce.ProcIo = proc_pid_io
    if rawCgroup is not None:
        pmce.RawCgroup = rawCgroup
    if pmce.RawCgroup is not None:
        pmce._procCgroups = procfs.proc_cgroup.ProcPidCgroups(pmce.RawCgroup)
        metrics = generate_proc_pid_cgroup_metrics(pmce._procCgroups, pmce.CommonLabels)
        pmce.ProcPidCgroupMetrics = set(m.metric for m in metrics)
    else:
        pmce.ProcPidCgroupMetrics = set()
    if rawCmdline is not None:
        pmce.RawCmdline = rawCmdline
    if pmce.RawCmdline is not None:
        pmce._procCmdline = procfs.proc_cmdline.ProcPidCmdline(pmce.RawCmdline)
        pmce.ProcPidCmdlineMetric = generate_proc_pid_cmdline_metrics(
            pmce._procCmdline, pmce.CommonLabels
        ).metric
    else:
        pmce.ProcPidCmdlineMetric = ""
    if (
        prev_pmce is not None
        and pmce.ProcStat is not None
        and prev_pmce.ProcStat is not None
    ):
        pmce.DeltaUTime = pmce.ProcStat.UTime - prev_pmce.ProcStat.UTime
        pmce.DeltaSTime = pmce.ProcStat.STime - prev_pmce.ProcStat.STime


def generate_pmce_full_metrics(
    pmce: PidMetricsCacheEntry,
    ts: Optional[int] = None,
    prev_pmce: Optional[PidMetricsCacheEntry] = None,
    prev_ts: Optional[int] = None,
) -> List[Metric]:
    if ts is None:
        ts = prometheus_ts_to_ts(pmce.Timestamp)
    if prev_pmce is not None and prev_ts is None:
        prev_ts = prometheus_ts_to_ts(prev_pmce.Timestamp)
    # All the metrics corresponding to the current state:
    metrics = (
        generate_proc_pid_cgroup_metrics(pmce._procCgroups, pmce.CommonLabels, ts=ts)
        + [
            generate_proc_pid_cmdline_metrics(
                pmce._procCmdline, pmce.CommonLabels, ts=ts
            )
        ]
        + generate_proc_pid_io_metrics(pmce.ProcIo, pmce.CommonLabels, ts=ts)
        + generate_proc_pid_stat_metrics(
            pmce.ProcStat,
            pmce.CommonLabels,
            ts=ts,
            prev_proc_pid_stat=prev_pmce.ProcStat if prev_pmce is not None else None,
            prev_ts=prev_ts,
        )
        + generate_proc_pid_status_metrics(
            pmce.ProcStatus,
            pmce.CommonLabels,
            ts=ts,
        )
    )
    # Additional metrics required to clear out-of-scope pseudo-categorical ones:
    if prev_pmce is not None:
        changed_pseudo_categorical_metrics = (
            prev_pmce.ProcPidCgroupMetrics
            | set(
                [
                    prev_pmce.ProcPidCmdlineMetric,
                    prev_pmce.ProcPidStatStateMetric,
                    prev_pmce.ProcPidStatInfoMetric,
                    prev_pmce.ProcPidStatusInfoMetric,
                ]
            )
        ) - (
            pmce.ProcPidCgroupMetrics
            | set(
                [
                    pmce.ProcPidCmdlineMetric,
                    pmce.ProcPidStatStateMetric,
                    pmce.ProcPidStatInfoMetric,
                    pmce.ProcPidStatusInfoMetric,
                ]
            )
        )
        prom_ts = ts_to_prometheus_ts(ts)
        metrics.extend(
            Metric(metric=metric, val=0, ts=prom_ts)
            for metric in changed_pseudo_categorical_metrics
        )
    return metrics
