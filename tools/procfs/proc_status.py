#! /usr/bin/env python3

# handle /proc/[pid]/status
# reference: https://github.com/prometheus/procfs/blob/master/proc_status.go

import dataclasses
import enum
import os
import time
from typing import List, Optional

from metrics_common_test import TestdataProcfsRoot
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase, proc_pid_dir


@dataclasses.dataclass
class ProcStatus(ProcfsStructBase):
    # The process ID.
    PID: int = 0
    # The process name.
    Name: str = ""
    # Thread group ID.
    TGID: int = 0
    # Peak virtual memory size.
    VmPeak: int = 0
    # Virtual memory size.
    VmSize: int = 0
    # Locked memory size.
    VmLck: int = 0
    # Pinned memory size.
    VmPin: int = 0
    # Peak resident set size.
    VmHWM: int = 0
    # Resident set size (sum of RssAnnon RssFile and RssShmem).
    VmRSS: int = 0
    # Size of resident anonymous memory.
    RssAnon: int = 0
    # Size of resident file mappings.
    RssFile: int = 0
    # Size of resident shared memory.
    RssShmem: int = 0
    # Size of data segments.
    VmData: int = 0
    # Size of stack segments.
    VmStk: int = 0
    # Size of text segments.
    VmExe: int = 0
    # Shared library code size.
    VmLib: int = 0
    # Page table entries size.
    VmPTE: int = 0
    # Size of second-level page tables.
    VmPMD: int = 0
    # Swapped-out virtual memory size by anonymous private.
    VmSwap: int = 0
    # Size of hugetlb memory portions
    HugetlbPages: int = 0

    # Number of voluntary context switches.
    VoluntaryCtxtSwitches: int = 0
    # Number of involuntary context switches.
    NonVoluntaryCtxtSwitches: int = 0

    # UIDs of the process (Real, effective, saved set, and filesystem UIDs)
    UIDs: List[str] = dataclasses.field(default_factory=lambda: [""] * 4)
    # GIDs of the process (Real, effective, saved set, and filesystem GIDs)
    GIDs: List[str] = dataclasses.field(default_factory=lambda: [""] * 4)


class UidGidIndex(enum.IntEnum):
    REAL_ID = 0
    EFFECTIVE_ID = 1
    SAVED_ID = 2
    FILESYSTEM_ID = 3


PROC_STATUS_PARSE_FIELD_MAP = {
    # key: field in /proc/[pid]/status from https://man7.org/linux/man-pages/man5/proc.5.html
    # value: field in ProcStatus
    "Tgid": "TGID",
    "Name": "Name",
    "Uid": "UIDs",
    "Gid": "GIDs",
    "VmPeak": "VmPeak",
    "VmSize": "VmSize",
    "VmLck": "VmLck",
    "VmPin": "VmPin",
    "VmHWM": "VmHWM",
    "VmRSS": "VmRSS",
    "RssAnon": "RssAnon",
    "RssFile": "RssFile",
    "RssShmem": "RssShmem",
    "VmData": "VmData",
    "VmStk": "VmStk",
    "VmExe": "VmExe",
    "VmLib": "VmLib",
    "VmPTE": "VmPTE",
    "VmPMD": "VmPMD",
    "VmSwap": "VmSwap",
    "HugetlbPages": "HugetlbPages",
    "voluntary_ctxt_switches": "VoluntaryCtxtSwitches",
    "nonvoluntary_ctxt_switches": "NonVoluntaryCtxtSwitches",
}


def is_unit(unit: str) -> Optional[int]:
    if unit in {"kB"}:
        return 1024
    elif unit in {"mB"}:
        return 1024 * 1024
    elif unit in {"gB"}:
        return 1024 * 1024 * 1024
    return None


def load_proc_pid_status(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> ProcStatus:
    stat_path = os.path.join(
        proc_pid_dir(pid, tid=tid, procfs_root=procfs_root), "status"
    )
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    procStatus = ProcStatus(
        PID=pid if tid == 0 else tid,
        _ts=ts_to_prometheus_ts(ts),
    )
    with open(stat_path, "rt") as f:
        for line in f:
            i = line.index(":")
            if i <= 0:
                continue
            field, value = (
                PROC_STATUS_PARSE_FIELD_MAP.get(line[:i].strip()),
                line[i + 1 :].strip(),
            )
            if field is None or not value:
                continue
            val_type = type(getattr(procStatus, field))
            if val_type == str:
                pass
            elif val_type == list:
                value = value.split()
            elif val_type in {int, float}:
                words = value.split()
                value = int(words[0])
                if len(words) > 1:
                    unit = is_unit(words[1])
                    if unit is not None:
                        value *= unit
            setattr(procStatus, field, value)
    return procStatus
