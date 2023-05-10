#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import Optional

import procfs

from .common import Metric, sanitize_label_value, ts_to_prometheus_ts

PROC_PID_CMDLINE_METRIC_NAME = "proc_pid_cmdline"
proc_pid_cmdline_categorical_metric_names = [
    PROC_PID_CMDLINE_METRIC_NAME,
]


def generate_proc_pid_cmdline_metrics(
    proc_pid_cmdline: procfs.ProcPidCmdline,
    common_labels: str,
    ts: Optional[float] = None,
) -> Metric:
    if ts is None:
        ts = proc_pid_cmdline._ts
    labels_sep = "," if common_labels else ""
    prom_ts = ts_to_prometheus_ts(ts)
    return Metric(
        metric=(
            f"proc_pid_cmdline{{{common_labels}{labels_sep}"
            f'cmdline="{sanitize_label_value(proc_pid_cmdline.cmdline)}"'
            "}"
        ),
        val=1,
        ts=prom_ts,
    )
