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
    for metric_name, field_spec in [
        ("proc_net_dev_rx_bytes_total", "rx_bytes"),
        ("proc_net_dev_rx_packets_total", "rx_packets"),
        ("proc_net_dev_rx_errors_total", "rx_errors"),
        ("proc_net_dev_rx_dropped_total", "rx_dropped"),
        ("proc_net_dev_rx_fifo_total", "rx_fifo"),
        ("proc_net_dev_rx_frame_total", "rx_frame"),
        ("proc_net_dev_rx_compressed_total", "rx_compressed"),
        ("proc_net_dev_rx_multicast_total", "rx_multicast"),
        ("proc_net_dev_tx_bytes_total", "tx_bytes"),
        ("proc_net_dev_tx_packets_total", "tx_packets"),
        ("proc_net_dev_tx_errors_total", "tx_errors"),
        ("proc_net_dev_tx_dropped_total", "tx_dropped"),
        ("proc_net_dev_tx_fifo_total", "tx_fifo"),
        ("proc_net_dev_tx_collisions_total", "tx_collisions"),
        ("proc_net_dev_tx_carrier_total", "tx_carrier"),
        ("proc_net_dev_tx_compressed_total", "tx_compressed"),
    ]:
        metrics.append(
            Metric(
                metric=f'{metric_name}{{hostname="{_hostname}",job="{_job}",device="{device}"}}',
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
