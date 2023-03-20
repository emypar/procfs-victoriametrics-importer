#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import Optional

import procfs
from procfs.proc_status import UidGidIndex
from tools_common import sanitize_label_value

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(
    metrics_fn_map,
    [
        "Name",
        "TGID",
        ("UIDs", UidGidIndex.REAL_ID),
        ("UIDs", UidGidIndex.EFFECTIVE_ID),
        ("UIDs", UidGidIndex.SAVED_ID),
        ("UIDs", UidGidIndex.FILESYSTEM_ID),
        ("GIDs", UidGidIndex.REAL_ID),
        ("GIDs", UidGidIndex.EFFECTIVE_ID),
        ("GIDs", UidGidIndex.SAVED_ID),
        ("GIDs", UidGidIndex.FILESYSTEM_ID),
    ],
)
def proc_pid_status_info_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
    _val: int = 1,
) -> Metric:
    return Metric(
        metric=(
            "proc_pid_status_info{"
            + (
                common_labels
                + ("," if common_labels else "")
                + f'''name="{sanitize_label_value(procStatus.Name)}"'''
                + f''',tgid="{procStatus.TGID}"'''
                + f''',real_uid="{procStatus.UIDs[UidGidIndex.REAL_ID]}"'''
                + f''',effective_uid="{procStatus.UIDs[UidGidIndex.EFFECTIVE_ID]}"'''
                + f''',saved_uid="{procStatus.UIDs[UidGidIndex.SAVED_ID]}"'''
                + f''',filesystem_uid="{procStatus.UIDs[UidGidIndex.FILESYSTEM_ID]}"'''
                + f''',real_gid="{procStatus.GIDs[UidGidIndex.REAL_ID]}"'''
                + f''',effective_gid="{procStatus.GIDs[UidGidIndex.EFFECTIVE_ID]}"'''
                + f''',saved_gid="{procStatus.GIDs[UidGidIndex.SAVED_ID]}"'''
                + f''',filesystem_gid="{procStatus.GIDs[UidGidIndex.FILESYSTEM_ID]}"'''
            )
            + "}"
        ),
        val=_val,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmPeak")
def proc_pid_status_vm_peak_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_peak" + "{" + common_labels + "}",
        val=procStatus.VmPeak,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmSize")
def proc_pid_status_vm_peak_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_size" + "{" + common_labels + "}",
        val=procStatus.VmSize,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmLck")
def proc_pid_status_vm_lck_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_lck" + "{" + common_labels + "}",
        val=procStatus.VmLck,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmPin")
def proc_pid_status_vm_lck_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_pin" + "{" + common_labels + "}",
        val=procStatus.VmPin,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmHWM")
def proc_pid_status_vm_hwm_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_hwm" + "{" + common_labels + "}",
        val=procStatus.VmHWM,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmRSS")
def proc_pid_status_vm_rss_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_rss" + "{" + common_labels + "}",
        val=procStatus.VmRSS,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "RssAnon")
def proc_pid_status_vm_rss_anon_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_rss_anon" + "{" + common_labels + "}",
        val=procStatus.RssAnon,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "RssFile")
def proc_pid_status_vm_rss_file_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_rss_file" + "{" + common_labels + "}",
        val=procStatus.RssFile,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "RssShmem")
def proc_pid_status_vm_rss_shmem_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_rss_shmem" + "{" + common_labels + "}",
        val=procStatus.RssShmem,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmData")
def proc_pid_status_vm_data_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_data" + "{" + common_labels + "}",
        val=procStatus.VmData,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmStk")
def proc_pid_status_vm_stk_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_stk" + "{" + common_labels + "}",
        val=procStatus.VmStk,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmExe")
def proc_pid_status_vm_exe_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_exe" + "{" + common_labels + "}",
        val=procStatus.VmExe,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmLib")
def proc_pid_status_vm_lib_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_lib" + "{" + common_labels + "}",
        val=procStatus.VmLib,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmPTE")
def proc_pid_status_vm_pte_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_pte" + "{" + common_labels + "}",
        val=procStatus.VmPTE,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmPMD")
def proc_pid_status_vm_pmd_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_pmd" + "{" + common_labels + "}",
        val=procStatus.VmPMD,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VmSwap")
def proc_pid_status_vm_swap_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_vm_swap" + "{" + common_labels + "}",
        val=procStatus.VmSwap,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "HugetlbPages")
def proc_pid_status_hugetbl_pages_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_hugetbl_pages" + "{" + common_labels + "}",
        val=procStatus.HugetlbPages,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "VoluntaryCtxtSwitches")
def proc_pid_status_voluntary_ctxt_switches_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_voluntary_ctxt_switches" + "{" + common_labels + "}",
        val=procStatus.VoluntaryCtxtSwitches,
        ts=ts if ts is not None else procStatus._ts,
    )


@register_metrics_fn(metrics_fn_map, "NonVoluntaryCtxtSwitches")
def proc_pid_status_nonvoluntary_ctxt_switches_metric(
    procStatus: procfs.ProcStatus,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_status_nonvoluntary_ctxt_switches" + "{" + common_labels + "}",
        val=procStatus.NonVoluntaryCtxtSwitches,
        ts=ts if ts is not None else procStatus._ts,
    )
