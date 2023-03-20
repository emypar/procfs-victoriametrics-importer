# /usr/bin/env python3

#  https://github.com/prometheus/procfs like module


import dataclasses
import os
from typing import List, Tuple, Union

from metrics_common_test import TestdataProcfsRoot
from tools_common import StructBase

ProcfsStructSingleVal = Union[int, float, str]
ProcfsStructVal = Union[ProcfsStructSingleVal, List[ProcfsStructSingleVal]]
ProcfsStructFieldSpec = Union[str, Tuple[str, int]]


@dataclasses.dataclass
class ProcfsStructBase(StructBase):
    _ts: int = 0

    def get_field(self, field_spec: ProcfsStructFieldSpec) -> ProcfsStructVal:
        if isinstance(field_spec, tuple):
            return getattr(self, field_spec[0])[field_spec[1]]
        else:
            return getattr(self, field_spec)

    def set_field(self, field_spec: ProcfsStructFieldSpec, val: ProcfsStructVal):
        if isinstance(field_spec, tuple):
            getattr(self, field_spec[0])[field_spec[1]] = val
        else:
            setattr(self, field_spec, val)


def proc_pid_dir(pid: int, tid: int = 0, procfs_root: str = TestdataProcfsRoot) -> str:
    if tid == 0:
        return os.path.join(procfs_root, str(pid))
    else:
        return os.path.join(procfs_root, str(pid), "task", str(tid))
