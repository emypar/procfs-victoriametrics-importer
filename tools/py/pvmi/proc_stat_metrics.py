#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestClktckSec, TestHostname, TestJob

from .common import Metric, ts_to_prometheus_ts


def proc_stat_metrics(
    proc_stat: procfs.Stat2,
    ts: Optional[float] = None,
    prev_proc_stat: Optional[procfs.Stat2] = None,
    prev_ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    if ts is None:
        ts = proc_stat._ts
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = [
        Metric(
            metric=f'proc_stat_boot_time{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.BootTime,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_irq_total_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.IRQTotal,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_softirq_total_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.SoftIRQTotal,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_context_switches_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.ContextSwitches,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_process_created_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.ProcessCreated,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_process_running_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.ProcessesRunning,
            ts=prom_ts,
        ),
        Metric(
            metric=f'proc_stat_process_blocked_count{{hostname="{_hostname}",job="{_job}"}}',
            val=proc_stat.ProcessesBlocked,
            ts=prom_ts,
        ),
    ]

    if prev_ts is None and prev_proc_stat is not None:
        prev_ts = prev_proc_stat._ts
    if prev_ts is not None:
        delta_seconds = ts - prev_ts
    for i, cpu_stat in enumerate(proc_stat.CPU):
        cpu = cpu_stat.Cpu
        if cpu == -1:
            cpu = "all"
        prev_cpu_stat = prev_proc_stat.CPU[i] if prev_proc_stat is not None else None
        for field, cpu_time_type in [
            (".User", "user"),
            (".Nice", "nice"),
            (".System", "system"),
            (".Idle", "idle"),
            (".Iowait", "iowait"),
            (".IRQ", "irq"),
            (".SoftIRQ", "softirq"),
            (".Steal", "steal"),
            (".Guest", "guest"),
            (".GuestNice", "guest_nice"),
        ]:
            val = cpu_stat.get_field(field)
            metrics.append(
                Metric(
                    metric=f'proc_stat_cpu_time_seconds{{hostname="{_hostname}",job="{_job}",cpu="{cpu}",type="{cpu_time_type}"}}',
                    val=val * TestClktckSec,
                    ts=prom_ts,
                    valfmt=".06f",
                )
            )
            if prev_cpu_stat is not None:
                prev_val = prev_cpu_stat.get_field(field)
                metrics.append(
                    Metric(
                        metric=f'proc_stat_cpu_time_pct{{hostname="{_hostname}",job="{_job}",cpu="{cpu}",type="{cpu_time_type}"}}',
                        val=(val - prev_val) * TestClktckSec / delta_seconds * 100.0,
                        ts=prom_ts,
                        valfmt=".02f",
                    )
                )
    return metrics
