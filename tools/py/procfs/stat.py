# /usr/bin/env python3

# handle /proc/stat
# reference: https:#github.com/prometheus/procfs/blob/master/stat.go

import dataclasses
import os
import re
import time
from typing import Dict, List

from metrics_common_test import TestClktckSec, TestdataProcfsRoot
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase


def round_float(f: float, dp: int = 7) -> float:
    ten_power = 10**dp
    return int(f * ten_power) / float(ten_power)


@dataclasses.dataclass
class CPUStat(ProcfsStructBase):
    User: float = 0.0
    Nice: float = 0.0
    System: float = 0.0
    Idle: float = 0.0
    Iowait: float = 0.0
    IRQ: float = 0.0
    SoftIRQ: float = 0.0
    Steal: float = 0.0
    Guest: float = 0.0
    GuestNice: float = 0.0

    def __init__(
        self,
        user: str,
        nice: str,
        system: str,
        idle: str,
        iowait: str,
        irq: str,
        softirq: str,
        steal: str = "0",
        guest: str = "0",
        guest_nice: str = "0",
    ):
        self.User = round_float(float(user) * TestClktckSec)
        self.Nice = round_float(float(nice) * TestClktckSec)
        self.System = round_float(float(system) * TestClktckSec)
        self.Idle = round_float(float(idle) * TestClktckSec)
        self.Iowait = round_float(float(iowait) * TestClktckSec)
        self.SoftIRQ = round_float(float(softirq) * TestClktckSec)
        self.Steal = round_float(float(steal) * TestClktckSec)
        self.Guest = round_float(float(guest) * TestClktckSec)
        self.GuestNice = round_float(float(guest_nice) * TestClktckSec)


@dataclasses.dataclass
class SoftIRQStat(ProcfsStructBase):
    Hi: int = 0
    Timer: int = 0
    NetTx: int = 0
    NetRx: int = 0
    Block: int = 0
    BlockIoPoll: int = 0
    Tasklet: int = 0
    Sched: int = 0
    Hrtimer: int = 0
    Rcu: int = 0


@dataclasses.dataclass
class Stat(ProcfsStructBase):
    # Boot time in seconds since the Epoch.
    BootTime: int = 0
    # Summed up cpu statistics.
    CPUTotal: CPUStat = None
    # Per-CPU statistics.
    CPU: Dict[int, CPUStat] = dataclasses.field(default_factory=lambda: {})
    # Number of times interrupts were handled, which contains numbered and unnumbered IRQs.
    IRQTotal: int = 0
    # Number of times a numbered IRQ was triggered.
    IRQ: List[int] = dataclasses.field(default_factory=lambda: [])
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
    # Detailed softirq statistics.
    SoftIRQ: SoftIRQStat = None


def load_stat(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
    _include_irq_details: bool = False,
) -> Stat:
    stat_path = os.path.join(procfs_root, "stat")
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    stat = Stat(_ts=ts_to_prometheus_ts(ts))
    with open(stat_path, "rt") as f:
        for line in f:
            words = line.split()
            if words[0] == "cpu":
                stat.CPUTotal = CPUStat(*words[1:])
                continue
            m = re.match(r"cpu(\d+)$", words[0])
            if m is not None:
                cpuN = int(m.group(1))
                stat.CPU[cpuN] = CPUStat(*words[1:])
                continue
            if words[0] == "btime":
                stat.BootTime = int(words[1])
                continue
            if words[0] == "intr":
                stat.IRQTotal = int(words[1])
                if _include_irq_details:
                    stat.IRQ = list(int(w) for w in words[2:])
                continue
            if words[0] == "ctxt":
                stat.ContextSwitches = int(words[1])
                continue
            if words[0] == "processes":
                stat.ProcessCreated = int(words[1])
                continue
            if words[0] == "procs_running":
                stat.ProcessesRunning = int(words[1])
                continue
            if words[0] == "procs_blocked":
                stat.ProcessesBlocked = int(words[1])
                continue
            if words[0] == "softirq":
                stat.SoftIRQTotal = int(words[1])
                if _include_irq_details:
                    stat.SoftIRQ = SoftIRQStat(*[int(w) for w in words[2:]])
                continue

    return stat
