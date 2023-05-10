#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import dataclasses
from typing import List, Optional

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot
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
    ProcStat: List[Optional[procfs.ProcStat]] = dataclasses.field(
        default_factory=lambda: [None, None]
    )
    CrtProcStatIndex: int = 1
    ProcStatus: List[procfs.ProcStatus] = dataclasses.field(
        default_factory=lambda: [None, None]
    )
    CrtProcStatusIndex: int = 1
    ProcIo: List[procfs.ProcIO] = dataclasses.field(
        default_factory=lambda: [None, None]
    )
    CrtProcIoIndex: int = 1
    # Cache raw data from /proc, for info that is expensive to parse and which
    # doesn't change often:
    RawCgroup: bytes = b""
    RawCmdline: bytes = b""
    # Cache %CPU time, needed for delta strategy. If either of the % changes
    # then all 3 (system, user, total) are being generated. A negative value
    # stands for invalid (this information is available at the end of the 2nd
    # scan).
    PrevUTimePct: float = -1.0
    PrevSTimePct: float = -1.0
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

PmceStatFieldNameToIndexMap = {
    "ProcStat": "CrtProcStatIndex",
    "ProcStatus": "CrtProcStatusIndex",
    "ProcIo": "CrtProcIoIndex",
}


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
        ProcStat=[proc_pid_stat, None],
        CrtProcStatIndex=0,
        ProcStatus=[proc_pid_status, None],
        CrtProcStatusIndex=0,
        ProcIo=[proc_pid_io, None],
        CrtProcIoIndex=0,
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
    prev_prom_ts: Optional[int] = None,
):
    if proc_pid_stat is not None:
        pmce.ProcStat[pmce.CrtProcStatIndex] = proc_pid_stat
    if pmce.ProcStat[pmce.CrtProcStatIndex] is not None:
        metrics = generate_proc_pid_stat_metrics(
            pmce.ProcStat[pmce.CrtProcStatIndex], pmce.CommonLabels
        )
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
        pmce.ProcStatus[pmce.CrtProcStatusIndex] = proc_pid_status
    if pmce.ProcStatus[pmce.CrtProcStatusIndex] is not None:
        metrics = generate_proc_pid_status_metrics(
            pmce.ProcStatus[pmce.CrtProcStatusIndex], pmce.CommonLabels
        )
        for m in metrics:
            m_name = m.name()
            if m_name == PROC_PID_STATUS_INFO_METRIC_NAME:
                pmce.ProcPidStatusInfoMetric = m.metric
    else:
        pmce.ProcPidStatusInfoMetric = ""
    if proc_pid_io is not None:
        pmce.ProcIo[pmce.CrtProcIoIndex] = proc_pid_io
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
        prev_prom_ts is not None
        and pmce.ProcStat[pmce.CrtProcStatIndex] is not None
        and pmce.ProcStat[1 - pmce.CrtProcStatIndex] is not None
    ):
        delta_sec = prometheus_ts_to_ts(pmce.Timestamp) - prometheus_ts_to_ts(
            prev_prom_ts
        )
        pmce.PrevUTimePct = (
            (
                pmce.ProcStat[pmce.CrtProcStatIndex].UTime
                - pmce.ProcStat[1 - pmce.CrtProcStatIndex].UTime
            )
            * TestClktckSec
            / delta_sec
            * 100
        )
        pmce.PrevSTimePct = (
            (
                pmce.ProcStat[pmce.CrtProcStatIndex].STime
                - pmce.ProcStat[1 - pmce.CrtProcStatIndex].STime
            )
            * TestClktckSec
            / delta_sec
            * 100
        )


def generate_pmce_full_metrics(
    pmce: PidMetricsCacheEntry,
    prom_ts: Optional[int] = None,
    prev_pmce: Optional[PidMetricsCacheEntry] = None,
    prev_prom_ts: Optional[int] = None,
) -> List[Metric]:
    if prom_ts is None:
        prom_ts = pmce.Timestamp
    if prev_prom_ts is None and prev_pmce is not None:
        prev_prom_ts = prev_pmce.Timestamp
    ts = prometheus_ts_to_ts(prom_ts)
    # All the metrics corresponding to the current state:
    metrics = []
    # Clear pseudo-categorical:
    if prev_pmce is not None:
        if prev_pmce.ProcPidCgroupMetrics:
            metrics.extend(
                Metric(metric=m, val=0, ts=prom_ts)
                for m in prev_pmce.ProcPidCgroupMetrics
                if not pmce.ProcPidCgroupMetrics or m not in pmce.ProcPidCgroupMetrics
            )
        if (
            prev_pmce.ProcPidCmdlineMetric != ""
            and prev_pmce.ProcPidCmdlineMetric != pmce.ProcPidCmdlineMetric
        ):
            metrics.append(
                Metric(metric=prev_pmce.ProcPidCmdlineMetric, val=0, ts=prom_ts)
            )
        if (
            prev_pmce.ProcPidStatStateMetric != ""
            and prev_pmce.ProcPidStatStateMetric != pmce.ProcPidStatStateMetric
        ):
            metrics.append(
                Metric(metric=prev_pmce.ProcPidStatStateMetric, val=0, ts=prom_ts)
            )
        if (
            prev_pmce.ProcPidStatInfoMetric != ""
            and prev_pmce.ProcPidStatInfoMetric != pmce.ProcPidStatInfoMetric
        ):
            metrics.append(
                Metric(metric=prev_pmce.ProcPidStatInfoMetric, val=0, ts=prom_ts)
            )
        if (
            prev_pmce.ProcPidStatusInfoMetric != ""
            and prev_pmce.ProcPidStatusInfoMetric != pmce.ProcPidStatusInfoMetric
        ):
            metrics.append(
                Metric(metric=prev_pmce.ProcPidStatusInfoMetric, val=0, ts=prom_ts)
            )

    if pmce.ProcPidCgroupMetrics:
        metrics.extend(
            Metric(metric=m, val=1, ts=prom_ts) for m in pmce.ProcPidCgroupMetrics
        )
    if pmce.ProcPidCmdlineMetric != "":
        metrics.append(Metric(metric=pmce.ProcPidCmdlineMetric, val=1, ts=prom_ts))
    metrics.extend(
        generate_proc_pid_io_metrics(
            pmce.ProcIo[pmce.CrtProcIoIndex], pmce.CommonLabels, ts=ts
        )
    )
    metrics.extend(
        generate_proc_pid_stat_metrics(
            pmce.ProcStat[pmce.CrtProcStatIndex],
            pmce.CommonLabels,
            ts=ts,
            prev_proc_pid_stat=pmce.ProcStat[1 - pmce.CrtProcStatIndex]
            if prev_prom_ts is not None
            else None,
            prev_ts=prometheus_ts_to_ts(prev_prom_ts)
            if prev_prom_ts is not None
            else None,
        )
    )
    metrics.extend(
        generate_proc_pid_status_metrics(
            pmce.ProcStatus[pmce.CrtProcStatusIndex],
            pmce.CommonLabels,
            ts=ts,
        )
    )
    return metrics
