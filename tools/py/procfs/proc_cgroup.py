#! /usr/bin/env python3

# handle /proc/[pid]/cgroup
# reference: https:#github.com/prometheus/procfs/blob/master/proc_cgroup.go

import dataclasses
import os
import time
from typing import List, Optional

from metrics_common_test import TestdataProcfsRoot

from .common import ProcfsStructBase, proc_pid_dir


@dataclasses.dataclass
class ProcCgroup(ProcfsStructBase):
    # HierarchyID that can be matched to a named hierarchy using /proc/cgroups. Cgroups V2 only has one
    # hierarchy, so HierarchyID is always 0. For cgroups v1 this is a unique ID number
    HierarchyID: int = 0
    # Controllers using this hierarchy of processes. Controllers are also known as subsystems. For
    # Cgroups V2 this may be empty, as all active controllers use the same hierarchy
    Controllers: List[str] = dataclasses.field(default_factory=list)
    # Path of this control group, relative to the mount point of the cgroupfs representing this specific
    # hierarchy
    Path: Optional[str] = None

    def __init__(self, line: str):
        # hierarchyID:[controller1,controller2]:path
        words = line.strip().split(":", 3)
        if len(words) < 3:
            return
        self.HierarchyID = int(words[0])
        self.Controllers = words[1].split(",")
        self.Path = words[2]


@dataclasses.dataclass
class ProcPidCgroups(ProcfsStructBase):
    cgroups: List[ProcCgroup] = dataclasses.field(default_factory=list)
    raw: bytes = b""

    def __init__(self, raw: bytes):
        self.cgroups = []
        self.raw = raw
        for line in str(raw, "utf-8").splitlines():
            cgroup = ProcCgroup(line)
            if cgroup.Path is not None:
                self.cgroups.append(cgroup)


def load_proc_pid_cgroups(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> ProcPidCgroups:
    stat_path = os.path.join(
        proc_pid_dir(pid, tid=tid, procfs_root=procfs_root), "cgroup"
    )
    with open(stat_path, "rb") as f:
        raw = f.read()
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    procPidCgroups = ProcPidCgroups(raw)
    procPidCgroups._ts = ts
    return procPidCgroups
