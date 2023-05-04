#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import List, Optional

import procfs

from .common import Metric, ts_to_prometheus_ts

metrics_fn_map = {}


def generate_proc_pid_io_metrics(
    proc_pid_io: procfs.ProcIO,
    common_labels: str = "",
    ts: Optional[int] = None,
) -> List[Metric]:
    if ts is None:
        ts = proc_pid_io._ts
    labels_sep = "," if common_labels else ""
    prom_ts = ts_to_prometheus_ts(ts)
    return [
        Metric(
            metric=f"proc_pid_io_rcar{{{common_labels}}}",
            val=proc_pid_io.RChar,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_wcar{{{common_labels}}}",
            val=proc_pid_io.WChar,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_syscr{{{common_labels}}}",
            val=proc_pid_io.SyscR,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_syscw{{{common_labels}}}",
            val=proc_pid_io.SyscW,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_readbytes{{{common_labels}}}",
            val=proc_pid_io.ReadBytes,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_writebytes{{{common_labels}}}",
            val=proc_pid_io.WriteBytes,
            ts=prom_ts,
        ),
        Metric(
            metric=f"proc_pid_io_cancelled_writebytes{{{common_labels}}}",
            val=proc_pid_io.CancelledWriteBytes,
            ts=prom_ts,
        ),
    ]
