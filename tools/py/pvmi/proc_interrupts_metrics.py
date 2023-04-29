#! /usr/bin/env python3

# support for pvmi/proc_interrupts_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map)
def proc_interrupts_metrics(
    interrupts: procfs.Interrupts,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
    _clear_pseudo_only: bool = False,
) -> List[Metric]:
    metrics = []
    for irq, interrupt in interrupts.items():
        if ts is None:
            ts = interrupt._ts
        if not _clear_pseudo_only:
            metrics.extend(
                Metric(
                    metric=(
                        f'proc_interrupts_total{{hostname="{_hostname}",job="{_job}",interrupt="{irq}",cpu="{cpu}"}}'
                    ),
                    val=val,
                    ts=ts,
                )
                for (cpu, val) in enumerate(interrupt.Values)
            )
        metrics.append(
            Metric(
                metric=(
                    f'proc_interrupts_info{{hostname="{_hostname}",job="{_job}",interrupt="{irq}",devices="{interrupt.Devices}",info="{interrupt.Info}"}}'
                ),
                val=0 if _clear_pseudo_only else 1,
                ts=ts,
            )
        )
    return metrics
