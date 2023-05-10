#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import List, Optional

import procfs

from .common import Metric, ts_to_prometheus_ts

PROC_PID_CGROUP_METRIC_NAME = "proc_pid_cgroup"
proc_pid_cgroup_categorical_metric_names = [PROC_PID_CGROUP_METRIC_NAME]


def generate_proc_pid_cgroup_metrics(
    proc_pid_croups: procfs.ProcPidCgroups,
    common_labels: str = "",
    ts: Optional[float] = None,
) -> List[Metric]:
    if ts is None:
        ts = proc_pid_croups._ts
    labels_sep = "," if common_labels else ""
    prom_ts = ts_to_prometheus_ts(ts)
    return [
        Metric(
            metric=(
                f"proc_pid_cgroup{{{common_labels}"
                f'{labels_sep}hierarchy_id="{cgroup.HierarchyID}"'
                f',controller="{controller}"'
                f',path="{cgroup.Path}"'
                "}"
            ),
            val=1,
            ts=prom_ts,
        )
        for cgroup in proc_pid_croups.cgroups
        for controller in cgroup.Controllers
    ]
