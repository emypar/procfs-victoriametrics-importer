#! /usr/bin/env python3

# support for pvmi/proc_interrupts_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric, ts_to_prometheus_ts


def proc_interrupts_metrics(
    interrupts: procfs.Interrupts,
    ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
    _clear_pseudo_only: bool = False,
) -> List[Metric]:
    metrics = []
    for irq, interrupt in interrupts.items():
        if ts is None:
            ts = interrupt._ts
        prom_ts = ts_to_prometheus_ts(ts)
        if not _clear_pseudo_only:
            metrics.extend(
                Metric(
                    metric=(
                        f'proc_interrupts_total{{hostname="{_hostname}",job="{_job}",interrupt="{irq}",cpu="{cpu}"}}'
                    ),
                    val=val,
                    ts=prom_ts,
                )
                for (cpu, val) in enumerate(interrupt.Values)
            )
        metrics.append(
            Metric(
                metric=(
                    f'proc_interrupts_info{{hostname="{_hostname}",job="{_job}",interrupt="{irq}",devices="{interrupt.Devices}",info="{interrupt.Info}"}}'
                ),
                val=0 if _clear_pseudo_only else 1,
                ts=prom_ts,
            )
        )
    return metrics


softirqs_field_to_irq_map = {
    "Hi": "hi",
    "Timer": "timer",
    "NetRx": "net_rx",
    "NetTx": "net_tx",
    "Block": "block",
    "IRQPoll": "irq_poll",
    "Tasklet": "tasklet",
    "Sched": "sched",
    "HRTimer": "hr_timer",
    "RCU": "rcu",
}


def proc_softirqs_metrics(
    softirqs: procfs.Softirqs,
    ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    metrics = []
    if ts is None:
        ts = softirqs._ts
    prom_ts = ts_to_prometheus_ts(ts)
    for field, softirq in softirqs_field_to_irq_map.items():
        metrics.extend(
            Metric(
                metric=(
                    f'proc_softirqs_total{{hostname="{_hostname}",job="{_job}",interrupt="{softirq}",cpu="{cpu}"}}'
                ),
                val=val,
                ts=prom_ts,
            )
            for (cpu, val) in enumerate(getattr(softirqs, field))
        )
    return metrics
