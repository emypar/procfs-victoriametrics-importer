# /usr/bin/env python3

# handle /proc/[pid]/stat
# reference: https://github.com/prometheus/procfs/blob/master/proc_stat.go

import dataclasses
import functools
import os
import time

from metrics_common_test import TestdataProcfsRoot, TestClktckSec
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase, proc_pid_dir


@dataclasses.dataclass
class ProcStat(ProcfsStructBase):
    # The process ID.
    PID: int = 0
    # The filename of the executable.
    Comm: str = ""
    # The process state.
    State: str = ""
    # The PID of the parent of this process.
    PPID: int = 0
    # The process group ID of the process.
    PGRP: int = 0
    # The session ID of the process.
    Session: int = 0
    # The controlling terminal of the process.
    TTY: int = 0
    # The ID of the foreground process group of the controlling terminal of
    # the process.
    TPGID: int = 0
    # The kernel flags word of the process.
    Flags: int = 0
    # The number of minor faults the process has made which have not required
    # loading a memory page from disk.
    MinFlt: int = 0
    # The number of minor faults that the process's waited-for children have
    # made.
    CMinFlt: int = 0
    # The number of major faults the process has made which have required
    # loading a memory page from disk.
    MajFlt: int = 0
    # The number of major faults that the process's waited-for children have
    # made.
    CMajFlt: int = 0
    # Amount of time that this process has been scheduled in user mode,
    # measured in clock ticks.
    UTime: int = 0
    # Amount of time that this process has been scheduled in kernel mode,
    # measured in clock ticks.
    STime: int = 0
    # Amount of time that this process's waited-for children have been
    # scheduled in user mode, measured in clock ticks.
    CUTime: int = 0
    # Amount of time that this process's waited-for children have been
    # scheduled in kernel mode, measured in clock ticks.
    CSTime: int = 0
    # For processes running a real-time scheduling policy, this is the negated
    # scheduling priority, minus one.
    Priority: int = 0
    # The nice value, a value in the range 19 (low priority) to -20 (high
    # priority).
    Nice: int = 0
    # Number of threads in this process.
    NumThreads: int = 0
    # The time the process started after system boot, the value is expressed
    # in clock ticks.
    Starttime: int = 0
    # Virtual memory size in bytes.
    VSize: int = 0
    # Resident set size in pages.
    RSS: int = 0
    # Soft limit in bytes on the rss of the process.
    RSSLimit: int = 0
    # CPU number last executed on.
    Processor: int = 0
    # Real-time scheduling priority, a number in the range 1 to 99 for processes
    # scheduled under a real-time policy, or 0, for non-real-time processes.
    RTPriority: int = 0
    # Scheduling policy.
    Policy: int = 0
    # Aggregated block I/O delays, measured in clock ticks (centiseconds).
    DelayAcctBlkIOTicks: int = 0


# Parse /proc/<pid>/stat into procfs.ProcStat, see:
# https://github.com/prometheus/procfs/blob/9ccf0f883747d9cab01c41d59dc5c8c29695cd86/proc_stat.go#L125
PROC_STAT_INDEX_TO_FIELD_NAME_MAP = {
    i: f
    for i, f in enumerate(
        [
            "PID",
            "Comm",
            "State",
            "PPID",
            "PGRP",
            "Session",
            "TTY",
            "TPGID",
            "Flags",
            "MinFlt",
            "CMinFlt",
            "MajFlt",
            "CMajFlt",
            "UTime",
            "STime",
            "CUTime",
            "CSTime",
            "Priority",
            "Nice",
            "NumThreads",
            None,
            "Starttime",
            "VSize",
            "RSS",
            "RSSLimit",
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            None,
            "Processor",
            "RTPriority",
            "Policy",
            "DelayAcctBlkIOTicks",
        ]
    )
    if f is not None
}


@functools.lru_cache(maxsize=None)
def load_proc_pid_stat(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> ProcStat:
    stat_path = os.path.join(
        proc_pid_dir(pid, tid=tid, procfs_root=procfs_root), "stat"
    )
    with open(stat_path, "rt") as f:
        line = f.readline().strip()
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    # Locate comm parenthesis; beware of ((comm)) case:
    start_comm_mark, end_comm_mark = " (", ") "
    comm_s = line.find(start_comm_mark) + len(start_comm_mark)
    comm_e = line.find(end_comm_mark, comm_s)
    comm = line[comm_s:comm_e]
    pid = line[: comm_s - len(start_comm_mark)].strip()
    fields = [pid, comm] + line[comm_e + len(end_comm_mark) :].split()
    procStat = ProcStat(_ts=ts_to_prometheus_ts(ts))
    for i, field_name in PROC_STAT_INDEX_TO_FIELD_NAME_MAP.items():
        field_type = type(getattr(procStat, field_name))
        setattr(procStat, field_name, field_type(fields[i]))
    return procStat
