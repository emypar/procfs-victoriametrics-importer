#! /usr/bin/env python3

# support for pvmi/proc_net_dev_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import (
    HIGH32_METRICS_NAME_SUFFIX,
    LOW32_METRICS_NAME_SUFFIX,
    Metric,
    ts_to_prometheus_ts,
)

UNIT64_ROLLOVER_CORRECTION = 1 << 64


def proc_net_dev_line_metrics(
    net_dev_line: procfs.NetDevLine,
    ts: Optional[float] = None,
    prev_net_dev_line: Optional[procfs.NetDevLine] = None,
    prev_ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    if ts is None:
        ts = net_dev_line._ts
    prom_ts = ts_to_prometheus_ts(ts)
    metrics = []
    device = net_dev_line.name
    if prev_net_dev_line is not None:
        if prev_ts is None:
            prev_ts = prev_net_dev_line._ts
        d_time = ts - prev_ts
        for metric_name, field_spec, side in [
            ("proc_net_dev_bps", ".rx_bytes", "rx"),
            ("proc_net_dev_bps", ".tx_bytes", "tx"),
        ]:
            d_val = net_dev_line.get_field(field_spec) - prev_net_dev_line.get_field(
                field_spec
            )
            if d_val < 0:
                d_val += UNIT64_ROLLOVER_CORRECTION
            metrics.append(
                Metric(
                    metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}",device="{device}",side="{side}"}}',
                    val=int(d_val / d_time) * 8,
                    ts=prom_ts,
                )
            )
    for metric_name, field_spec, side in [
        ("proc_net_dev_bytes_total", ".rx_bytes", "rx"),
        ("proc_net_dev_bytes_total", ".tx_bytes", "tx"),
    ]:
        val = net_dev_line.get_field(field_spec)
        metrics.extend(
            [
                Metric(
                    metric=f'{metric_name}{HIGH32_METRICS_NAME_SUFFIX}{{hostname="{_hostname}",job="{_job}",device="{device}",side="{side}"}}',
                    val=val >> 32,
                    ts=prom_ts,
                ),
                Metric(
                    metric=f'{metric_name}{LOW32_METRICS_NAME_SUFFIX}{{hostname="{_hostname}",job="{_job}",device="{device}",side="{side}"}}',
                    val=val & 0xFFFFFFFF,
                    ts=prom_ts,
                ),
            ]
        )
    for metric_name, field_spec, side in [
        ("proc_net_dev_packets_total", ".rx_packets", "rx"),
        ("proc_net_dev_errors_total", ".rx_errors", "rx"),
        ("proc_net_dev_dropped_total", ".rx_dropped", "rx"),
        ("proc_net_dev_fifo_total", ".rx_fifo", "rx"),
        ("proc_net_dev_frame_total", ".rx_frame", "rx"),
        ("proc_net_dev_compressed_total", ".rx_compressed", "rx"),
        ("proc_net_dev_multicast_total", ".rx_multicast", "rx"),
        ("proc_net_dev_packets_total", ".tx_packets", "tx"),
        ("proc_net_dev_errors_total", ".tx_errors", "tx"),
        ("proc_net_dev_dropped_total", ".tx_dropped", "tx"),
        ("proc_net_dev_fifo_total", ".tx_fifo", "tx"),
        ("proc_net_dev_collisions_total", ".tx_collisions", "tx"),
        ("proc_net_dev_carrier_total", ".tx_carrier", "tx"),
        ("proc_net_dev_compressed_total", ".tx_compressed", "tx"),
    ]:
        metrics.append(
            Metric(
                metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}",device="{device}",side="{side}"}}',
                val=net_dev_line.get_field(field_spec),
                ts=prom_ts,
            )
        )
    return metrics


def proc_net_dev_metrics(
    net_dev: procfs.NetDev,
    ts: Optional[float] = None,
    prev_net_dev: Optional[procfs.NetDev] = None,
    prev_ts: Optional[float] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    metrics = []
    for net_dev_line in net_dev.values():
        if isinstance(net_dev_line, procfs.NetDevLine):
            metrics.extend(
                proc_net_dev_line_metrics(
                    net_dev_line,
                    ts=ts,
                    prev_net_dev_line=(
                        prev_net_dev.get(net_dev_line.name)
                        if prev_net_dev is not None
                        else None
                    ),
                    prev_ts=prev_ts,
                    _hostname=_hostname,
                    _job=_job,
                )
            )
    return metrics
