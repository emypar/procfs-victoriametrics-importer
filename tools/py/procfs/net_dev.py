# /usr/bin/env python3

# handle /proc/net/dev
# reference: https:#github.com/prometheus/procfs/blob/master/net_dev.go

import dataclasses
import os
import sys
import time
from typing import Dict, Optional

from metrics_common_test import TestdataProcfsRoot
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase


@dataclasses.dataclass
class NetDevLine(ProcfsStructBase):
    # Must use the json file tags rather than the struct field name:
    name: str = ""
    rx_bytes: int = 0
    rx_packets: int = 0
    rx_errors: int = 0
    rx_dropped: int = 0
    rx_fifo: int = 0
    rx_frame: int = 0
    rx_compressed: int = 0
    rx_multicast: int = 0
    tx_bytes: int = 0
    tx_packets: int = 0
    tx_errors: int = 0
    tx_dropped: int = 0
    tx_fifo: int = 0
    tx_collisions: int = 0
    tx_carrier: int = 0
    tx_compressed: int = 0


NetDev = Dict[str, NetDevLine]


def load_net_dev(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> Optional[NetDev]:
    net_dev_path = os.path.join(procfs_root, "net/dev")
    ts = ts_to_prometheus_ts(
        os.stat(net_dev_path).st_mtime if _use_ts_from_file else time.time()
    )
    net_dev = dict()
    with open(net_dev_path, "rt") as f:
        n_line = 0
        for line in f:
            n_line += 1
            # Skip the 2 header lines:
            if n_line <= 2:
                continue
            i = line.find(":")
            if i < 0:
                print(f"{net_dev_path}:{n_line}: missing `:' ", file=sys.stderr)
                return None
            name = line[:i].strip()
            counters = [int(v) for v in line[i + 1 :].split()]
            net_dev[name] = NetDevLine(
                _ts=ts,
                name=name,
                rx_bytes=counters[0],
                rx_packets=counters[1],
                rx_errors=counters[2],
                rx_dropped=counters[3],
                rx_fifo=counters[4],
                rx_frame=counters[5],
                rx_compressed=counters[6],
                rx_multicast=counters[7],
                tx_bytes=counters[8],
                tx_packets=counters[9],
                tx_errors=counters[10],
                tx_dropped=counters[11],
                tx_fifo=counters[12],
                tx_collisions=counters[13],
                tx_carrier=counters[14],
                tx_compressed=counters[15],
            )
    return net_dev
