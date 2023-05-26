# /usr/bin/env python3

#  https://github.com/prometheus/procfs like module


import dataclasses
import os

from metrics_common_test import TestdataProcfsRoot
from tools_common import StructBase


@dataclasses.dataclass
class ProcfsStructBase(StructBase):
    _ts: float = 0


def proc_pid_dir(pid: int, tid: int = 0, procfs_root: str = TestdataProcfsRoot) -> str:
    if tid == 0:
        return os.path.join(procfs_root, str(pid))
    else:
        return os.path.join(procfs_root, str(pid), "task", str(tid))
