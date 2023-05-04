# /usr/bin/env python3

# handle /proc/[pid]/io
# reference: https://github.com/prometheus/procfs/blob/master/proc_io.go

import dataclasses
import os
import time

from metrics_common_test import TestdataProcfsRoot

from .common import ProcfsStructBase, proc_pid_dir


@dataclasses.dataclass
class ProcIO(ProcfsStructBase):
    # Chars read.
    RChar: int = 0
    # Chars written.
    WChar: int = 0
    # Read syscalls.
    SyscR: int = 0
    # Write syscalls.
    SyscW: int = 0
    # Bytes read.
    ReadBytes: int = 0
    # Bytes written.
    WriteBytes: int = 0
    # Bytes written, but taking into account truncation. See
    # Documentation/filesystems/proc.txt in the kernel sources for
    # detailed explanation.
    CancelledWriteBytes: int = 0


PROC_IO_PARSE_FIELD_MAP = {
    # key: field in /proc/[pid]/io
    # value: field in ProcIO
    "rchar": "RChar",
    "wchar": "WChar",
    "syscr": "SyscR",
    "syscw": "SyscW",
    "read_bytes": "ReadBytes",
    "write_bytes": "WriteBytes",
    "cancelled_write_bytes": "CancelledWriteBytes",
}


def load_proc_pid_io(
    pid: int,
    tid: int = 0,
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> ProcIO:
    stat_path = os.path.join(proc_pid_dir(pid, tid=tid, procfs_root=procfs_root), "io")
    ts = os.stat(stat_path).st_mtime if _use_ts_from_file else time.time()
    procIO = ProcIO(_ts=ts)
    with open(stat_path, "rt") as f:
        for line in f:
            i = line.index(":")
            if i <= 0:
                continue
            field, value = (
                PROC_IO_PARSE_FIELD_MAP.get(line[:i].strip()),
                line[i + 1 :].strip(),
            )
            if field is None or not value:
                continue
            val_type = type(getattr(procIO, field))
            setattr(procIO, field, val_type(value))
    return procIO
