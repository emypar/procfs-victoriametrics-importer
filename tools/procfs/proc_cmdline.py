# /usr/bin/env python3

# handle /proc/PID/cmdline

import dataclasses
import os
import time

from metrics_common_test import TestdataProcfsRoot
from tools_common import ts_to_prometheus_ts

from .common import ProcfsStructBase, proc_pid_dir


@dataclasses.dataclass
class ProcPidCmdline(ProcfsStructBase):
    cmdline: str = ""
    raw: bytes = b""

    def __init__(self, raw: bytes):
        self.raw = raw
        self.cmdline = str(raw.replace(b"\0", b" ").strip(), "utf-8")


def load_proc_pid_cmdline(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> ProcPidCmdline:
    stat_path = os.path.join(
        proc_pid_dir(pid, tid=tid, procfs_root=procfs_root), "cmdline"
    )
    with open(stat_path, "rb") as f:
        raw = f.read()
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    procPidCmdline = ProcPidCmdline(raw)
    procPidCmdline._ts = ts_to_prometheus_ts(ts)
    return procPidCmdline
