#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric


def proc_stat_metrics(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    prev_proc_stat: Optional[procfs.Stat] = None,
    prev_ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
    _full_metrics: bool = False,
) -> List[Metric]:
    metrics = []
    cpu_stat_map = dict(proc_stat.CPU)
    cpu_stat_map["all"] = proc_stat.CPUTotal
    if ts is None:
        ts = proc_stat._ts
    for cpu, cpu_stat in cpu_stat_map.items():
        for field, cpu_time_type in [
            ("User", "user"),
            ("Nice", "nice"),
            ("System", "system"),
            ("Idle", "idle"),
            ("Iowait", "iowait"),
            ("IRQ", "irq"),
            ("SoftIRQ", "softirq"),
            ("Steal", "steal"),
            ("Guest", "guest"),
            ("GuestNice", "guest_nice"),
        ]:
            val = cpu_stat.get_field(field)
            metrics.append(
                Metric(
                    metric=f'proc_stat_cpu_time_seconds{{hostname="{_hostname}",job="{_job}",cpu="{cpu}",type="{cpu_time_type}"}}',
                    val=val,
                    ts=ts,
                    valfmt=".06f",
                )
            )
    if prev_proc_stat is not None:
        prev_cpu_stat_map = dict(prev_proc_stat.CPU)
        prev_cpu_stat_map["all"] = prev_proc_stat.CPUTotal
        if prev_ts is None:
            prev_ts = prev_proc_stat._ts

        delta_seconds = (ts - prev_ts) / 1000.0  # Prom -> seconds
        for cpu in cpu_stat_map:
            cpu_stat = cpu_stat_map[cpu]
            prev_cpu_stat = prev_cpu_stat_map[cpu]
            for field, cpu_time_type in [
                ("User", "user"),
                ("Nice", "nice"),
                ("System", "system"),
                ("Idle", "idle"),
                ("Iowait", "iowait"),
                ("IRQ", "irq"),
                ("SoftIRQ", "softirq"),
                ("Steal", "steal"),
                ("Guest", "guest"),
                ("GuestNice", "guest_nice"),
            ]:
                val = cpu_stat.get_field(field)
                prev_val = prev_cpu_stat.get_field(field)
                if not _full_metrics and val == prev_val:
                    continue
                metrics.append(
                    Metric(
                        metric=f'proc_stat_cpu_time_pct{{hostname="{_hostname}",job="{_job}",cpu="{cpu}",type="{cpu_time_type}"}}',
                        val=(val - prev_val) / delta_seconds * 100.0,
                        ts=ts,
                        valfmt=".02f",
                    )
                )
    metrics.extend(
        [
            Metric(
                metric=f'proc_stat_boot_time{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.BootTime,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_irq_total_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.IRQTotal,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_softirq_total_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.SoftIRQTotal,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_context_switches_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.ContextSwitches,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_process_created_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.ProcessCreated,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_process_running_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.ProcessesRunning,
                ts=ts,
            ),
            Metric(
                metric=f'proc_stat_process_blocked_count{{hostname="{_hostname}",job="{_job}"}}',
                val=proc_stat.ProcessesBlocked,
                ts=ts,
            ),
        ]
    )
