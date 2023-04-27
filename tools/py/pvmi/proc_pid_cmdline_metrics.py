#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import Optional

import procfs
from tools_common import sanitize_label_value

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map)
def proc_pid_cmdline_metrics(
    procPidCmdline: procfs.ProcPidCmdline,
    common_labels: str,
    ts: Optional[int] = None,
    _val: int = 1,
) -> Metric:
    return Metric(
        metric=(
            "proc_pid_cmdline"
            + "{"
            + common_labels
            + f''',cmdline="{sanitize_label_value(procPidCmdline.cmdline)}"'''
            + "}"
        ),
        val=_val,
        ts=ts if ts is not None else procPidCmdline._ts,
    )
