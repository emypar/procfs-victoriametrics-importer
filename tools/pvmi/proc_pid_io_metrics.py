#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import Optional

import procfs

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map, "RChar")
def proc_pid_io_rcar_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_rcar" + "{" + common_labels + "}",
        val=procIO.RChar,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "WChar")
def proc_pid_io_wcar_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_wcar" + "{" + common_labels + "}",
        val=procIO.WChar,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "SyscR")
def proc_pid_io_syscr_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_syscr" + "{" + common_labels + "}",
        val=procIO.SyscR,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "SyscW")
def proc_pid_io_syscw_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_syscw" + "{" + common_labels + "}",
        val=procIO.SyscW,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "ReadBytes")
def proc_pid_io_readbytes_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_readbytes" + "{" + common_labels + "}",
        val=procIO.ReadBytes,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "WriteBytes")
def proc_pid_io_writebytes_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_writebytes" + "{" + common_labels + "}",
        val=procIO.WriteBytes,
        ts=ts if ts is not None else procIO._ts,
    )


@register_metrics_fn(metrics_fn_map, "CancelledWriteBytes")
def proc_pid_io_cancelled_writebytes_metric(
    procIO: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> Metric:
    return Metric(
        metric="proc_pid_io_cancelled_writebytes" + "{" + common_labels + "}",
        val=procIO.CancelledWriteBytes,
        ts=ts if ts is not None else procIO._ts,
    )
