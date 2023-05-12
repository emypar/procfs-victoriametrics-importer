# /usr/bin/env python3

# handle /proc/stat
# reference:

import dataclasses
import os
import time
from typing import List

from metrics_common_test import TestdataProcfsRoot

from .common import ProcfsStructBase


def round_float(f: float, dp: int = 7) -> float:
    ten_power = 10**dp
    return int(f * ten_power) / float(ten_power)


@dataclasses.dataclass
class CPUStat2(ProcfsStructBase):
    Cpu: int = 0
    User: int = 0
    Nice: int = 0
    System: int = 0
    Idle: int = 0
    Iowait: int = 0
    IRQ: int = 0
    SoftIRQ: int = 0
    Steal: int = 0
    Guest: int = 0
    GuestNice: int = 0

    def __init__(self, cpu: str, *args):
        self.Cpu = -1 if cpu == "cpu" else int(cpu[3:])
        for i, attr in enumerate(
            [
                "User",
                "Nice",
                "System",
                "Idle",
                "Iowait",
                "IRQ",
                "SoftIRQ",
                "Steal",
                "GuestNice",
            ]
        ):
            if len(args) < i + 1:
                break
            setattr(self, attr, int(args[i]))


@dataclasses.dataclass
class Stat2(ProcfsStructBase):
    # Boot time in seconds since the Epoch.
    BootTime: int = 0
    # Per-CPU statistics, sorted by .Cpu:.
    CPU: List[CPUStat2] = dataclasses.field(default_factory=list)
    # Number of times interrupts were handled, which contains numbered and unnumbered IRQs.
    IRQTotal: int = 0
    # Number of times a context switch happened.
    ContextSwitches: int = 0
    # Number of times a process was created.
    ProcessCreated: int = 0
    # Number of processes currently running.
    ProcessesRunning: int = 0
    # Number of processes currently blocked (waiting for IO).
    ProcessesBlocked: int = 0
    # Number of times a softirq was scheduled.
    SoftIRQTotal: int = 0


def load_stat2(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> Stat2:
    stat_path = os.path.join(procfs_root, "stat")
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    stat = Stat2(_ts=ts)
    with open(stat_path, "rt") as f:
        for line in f:
            words = line.split()
            if words[0].startswith("cpu"):
                stat.CPU.append(CPUStat2(*words))
            elif words[0] == "btime":
                stat.BootTime = int(words[1])
            elif words[0] == "intr":
                stat.IRQTotal = int(words[1])
            elif words[0] == "ctxt":
                stat.ContextSwitches = int(words[1])
            elif words[0] == "processes":
                stat.ProcessCreated = int(words[1])
            elif words[0] == "procs_running":
                stat.ProcessesRunning = int(words[1])
            elif words[0] == "procs_blocked":
                stat.ProcessesBlocked = int(words[1])
            elif words[0] == "softirq":
                stat.SoftIRQTotal = int(words[1])
    stat.CPU = sorted(stat.CPU, key=lambda cpuStat: cpuStat.Cpu)
    return stat
