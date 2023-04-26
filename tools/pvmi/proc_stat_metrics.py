#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


@register_metrics_fn(metrics_fn_map)
def proc_stat_cpu_metrics(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
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
    return metrics


@register_metrics_fn(metrics_fn_map, require_history=True)
def proc_stat_pcpu_metrics(
    proc_stat: procfs.Stat,
    prev_proc_stat: procfs.Stat,
    ts: Optional[int] = None,
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
    return metrics


@register_metrics_fn(metrics_fn_map)
def proc_stat_boot_time_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_boot_time{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.BootTime,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_irq_total_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_irq_total_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.IRQTotal,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_softirq_total_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_softirq_total_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.SoftIRQTotal,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_context_switches_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_context_switches_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.ContextSwitches,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_process_created_count_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_process_created_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.ProcessCreated,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_process_running_count_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_process_running_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.ProcessesRunning,
        ts=ts if ts is not None else proc_stat._ts,
    )


@register_metrics_fn(metrics_fn_map)
def proc_stat_process_blocked_count_count_metric(
    proc_stat: procfs.Stat,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> Metric:
    return Metric(
        metric=f'proc_stat_process_blocked_count{{hostname="{_hostname}",job="{_job}"}}',
        val=proc_stat.ProcessesBlocked,
        ts=ts if ts is not None else proc_stat._ts,
    )


def generate_all_metrics(
    proc_stat: procfs.Stat,
    prev_proc_stat: Optional[procfs.Stat] = None,
    ts: Optional[int] = None,
    prev_ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
    _full_metrics: bool = False,
) -> List[Metric]:
    all_metrics = []
    for metric_fn, require_history in metrics_fn_map.items():
        if require_history:
            if prev_proc_stat is None:
                continue
            metric_or_metrics = metric_fn(
                proc_stat,
                prev_proc_stat=prev_proc_stat,
                ts=ts,
                prev_ts=prev_ts,
                _hostname=_hostname,
                _job=_job,
                _full_metrics=_full_metrics,
            )
        else:
            metric_or_metrics = metric_fn(
                proc_stat,
                ts=ts,
                _hostname=_hostname,
                _job=_job,
            )
        if isinstance(metric_or_metrics, list):
            all_metrics.extend(metric_or_metrics)
        else:
            all_metrics.append(metric_or_metrics)
    return all_metrics
