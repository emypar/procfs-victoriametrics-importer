# /usr/bin/env python3

# handle /proc/interrupts
# reference: https://github.com/prometheus/procfs/blob/master/proc_interrupts.go

import dataclasses
import os
import time
from typing import Dict, List

from metrics_common_test import TestdataProcfsRoot
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase


@dataclasses.dataclass
class Interrupt(ProcfsStructBase):
    # Info is the type of interrupt.
    Info: str = ""
    # Devices is the name of the device that is located at that IRQ
    Devices: str = ""
    # Values is the number of interrupts per CPU.
    Values: List[str] = dataclasses.field(default_factory=list)


Interrupts = Dict[str, Interrupt]


def load_interrupts(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> Interrupts:
    interrupts_path = os.path.join(procfs_root, "interrupts")
    ts = ts_to_prometheus_ts(
        os.stat(interrupts_path).st_mtime if _use_ts_from_file else time.time()
    )
    interrupts = dict()
    with open(interrupts_path, "rt") as f:
        line = f.readline()
        n_cpus = len(line.split())
        for line in f:
            words = line.split()
            if len(words) < n_cpus + 1:
                continue
            irq, values = words[0][:-1], words[1 : n_cpus + 1]
            try:
                int(irq)
                info, devices = words[n_cpus + 1], " ".join(words[n_cpus + 2 :])
            except (ValueError, TypeError):
                info, devices = " ".join(words[n_cpus + 1 :]), ""
            interrupts[irq] = Interrupt(
                _ts=ts, Info=info, Devices=devices, Values=values
            )
    return interrupts
