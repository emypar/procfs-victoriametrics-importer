#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import List, Optional

import procfs
from procfs.proc_status import UidGidIndex

from .common import Metric, sanitize_label_value, ts_to_prometheus_ts

PROC_PID_STATUS_INFO_METRIC_NAME = "proc_pid_status_info"
proc_pid_status_categorical_metric_names = [
    PROC_PID_STATUS_INFO_METRIC_NAME,
]


def generate_proc_pid_status_metrics(
    proc_pid_status: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> List[Metric]:
    if ts is None:
        ts = proc_pid_status._ts
    labels_sep = "," if common_labels else ""
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = [
        Metric(
            metric=(
                f"proc_pid_status_info{{{common_labels}{labels_sep}"
                f'name="{sanitize_label_value(proc_pid_status.Name)}"'
                f',tgid="{proc_pid_status.TGID}"'
                f',real_uid="{proc_pid_status.UIDs[UidGidIndex.REAL_ID]}"'
                f',effective_uid="{proc_pid_status.UIDs[UidGidIndex.EFFECTIVE_ID]}"'
                f',saved_uid="{proc_pid_status.UIDs[UidGidIndex.SAVED_ID]}"'
                f',filesystem_uid="{proc_pid_status.UIDs[UidGidIndex.FILESYSTEM_ID]}"'
                f',real_gid="{proc_pid_status.GIDs[UidGidIndex.REAL_ID]}"'
                f',effective_gid="{proc_pid_status.GIDs[UidGidIndex.EFFECTIVE_ID]}"'
                f',saved_gid="{proc_pid_status.GIDs[UidGidIndex.SAVED_ID]}"'
                f',filesystem_gid="{proc_pid_status.GIDs[UidGidIndex.FILESYSTEM_ID]}"'
                "}"
            ),
            val=1,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_peak{{{common_labels}}}",
            val=proc_pid_status.VmPeak,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_size{{{common_labels}}}",
            val=proc_pid_status.VmSize,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_pin{{{common_labels}}}",
            val=proc_pid_status.VmPin,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_lck{{{common_labels}}}",
            val=proc_pid_status.VmLck,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_hwm{{{common_labels}}}",
            val=proc_pid_status.VmHWM,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_rss{{{common_labels}}}",
            val=proc_pid_status.VmRSS,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_rss_anon{{{common_labels}}}",
            val=proc_pid_status.RssAnon,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_rss_file{{{common_labels}}}",
            val=proc_pid_status.RssFile,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_rss_shmem{{{common_labels}}}",
            val=proc_pid_status.RssShmem,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_data{{{common_labels}}}",
            val=proc_pid_status.VmData,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_stk{{{common_labels}}}",
            val=proc_pid_status.VmStk,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_exe{{{common_labels}}}",
            val=proc_pid_status.VmExe,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_lib{{{common_labels}}}",
            val=proc_pid_status.VmLib,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_pte{{{common_labels}}}",
            val=proc_pid_status.VmPTE,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_pmd{{{common_labels}}}",
            val=proc_pid_status.VmPMD,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_vm_swap{{{common_labels}}}",
            val=proc_pid_status.VmSwap,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_hugetbl_pages{{{common_labels}}}",
            val=proc_pid_status.HugetlbPages,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_voluntary_ctxt_switches{{{common_labels}}}",
            val=proc_pid_status.VoluntaryCtxtSwitches,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_status_nonvoluntary_ctxt_switches{{{common_labels}}}",
            val=proc_pid_status.NonVoluntaryCtxtSwitches,
            ts=prom_ts,
        ),
    ]

    return metrics
