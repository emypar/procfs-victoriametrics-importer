#! /usr/bin/env python3

# support for pvmi/proc_net_dev_metrics.go testing

from typing import List, Optional

import procfs
from metrics_common_test import TestHostname, TestJob

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


def proc_net_dev_line_metrics(
    net_dev_line: procfs.NetDevLine,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    if ts is None:
        ts = net_dev_line._ts
    metrics = []
    device = net_dev_line.name
    for metric_name, field_spec, side, in [
        ("proc_net_dev_bytes_total", "rx_bytes", "rx"),
        ("proc_net_dev_packets_total", "rx_packets", "rx"),
        ("proc_net_dev_errors_total", "rx_errors", "rx"),
        ("proc_net_dev_dropped_total", "rx_dropped", "rx"),
        ("proc_net_dev_fifo_total", "rx_fifo", "rx"),
        ("proc_net_dev_frame_total", "rx_frame", "rx"),
        ("proc_net_dev_compressed_total", "rx_compressed", "rx"),
        ("proc_net_dev_multicast_total", "rx_multicast", "rx"),
        ("proc_net_dev_bytes_total", "tx_bytes", "tx"),
        ("proc_net_dev_packets_total", "tx_packets", "tx"),
        ("proc_net_dev_errors_total", "tx_errors", "tx"),
        ("proc_net_dev_dropped_total", "tx_dropped", "tx"),
        ("proc_net_dev_fifo_total", "tx_fifo", "tx"),
        ("proc_net_dev_collisions_total", "tx_collisions", "tx"),
        ("proc_net_dev_carrier_total", "tx_carrier", "tx"),
        ("proc_net_dev_compressed_total", "tx_compressed", "tx"),
    ]:
        metrics.append(
            Metric(
                metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}",device="{device}",side="{side}"}}',
                val=net_dev_line.get_field(field_spec),
                ts=ts,
            )
        )
    return metrics


@register_metrics_fn(metrics_fn_map)
def proc_net_dev_metrics(
    net_dev: procfs.NetDev,
    ts: Optional[int] = None,
    _hostname: str = TestHostname,
    _job: str = TestJob,
) -> List[Metric]:
    metrics = []
    for net_dev_line in net_dev.values():
        if isinstance(net_dev_line, procfs.NetDevLine):
            metrics.extend(
                proc_net_dev_line_metrics(
                    net_dev_line, ts=ts, _hostname=_hostname, _job=_job
                )
            )
    return metrics
