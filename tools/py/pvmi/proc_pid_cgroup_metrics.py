#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

from typing import List, Optional

import procfs

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map)
def proc_pid_cgroup_metrics(
    procPidGroups: procfs.ProcPidCgroups,
    common_labels: str = "",
    ts: Optional[int] = None,
    _val: int = 1,
) -> List[Metric]:
    return [
        Metric(
            metric=(
                "proc_pid_cgroup{"
                + (
                    common_labels
                    + ("," if common_labels else "")
                    + f'''hierarchy_id="{cgroup.HierarchyID}"'''
                    + f''',controller="{controller}"'''
                    + f''',path="{cgroup.Path}"'''
                )
                + "}"
            ),
            val=_val,
            ts=ts if ts is not None else procPidGroups._ts,
        )
        for cgroup in procPidGroups.cgroups
        for controller in cgroup.Controllers
    ]
