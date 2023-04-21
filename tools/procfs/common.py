# /usr/bin/env python3

#  https://github.com/prometheus/procfs like module


import dataclasses
import os
from typing import List, Tuple, Union, Any

from metrics_common_test import TestdataProcfsRoot
from tools_common import StructBase

ProcfsStructVal = Any
# 
ProcfsStructFieldSpec = str

@dataclasses.dataclass
class ProcfsStructBase(StructBase):
    _ts: int = 0

    def get_field_spec_list(self, recurse: bool = True) -> List[ProcfsStructFieldSpec]:
        field_spec_list = []            

        for field in self.__dataclass_fields__:
            if field.startswith("_"):
                continue
            val = getattr(self, field)
            if hasattr(val, "get_field_spec_list") and recurse:
                field_spec_list.extend(f"{field}.{f}" for f in val.get_field_spec_list(recurse=True))
            elif isinstance(val, (list, dict)) and len(val) > 0:
                i_range = range(len(val)) if isinstance(val, list) else val
                if not recurse:
                    field_spec_list.extend(
                        f"{field}[{i!r}]" 
                        for i in i_range
                    )
                else:
                    for i in i_range:
                        v_val = val[i]
                        if hasattr(v_val, "get_field_spec_list"):
                            field_spec_list.extend(f"{field}[{i!r}].{f}" for f in v_val.get_field_spec_list(recurse=True))
                        else:
                            field_spec_list.append(f"{field}[{i!r}]")
            else:
                field_spec_list.append(field)
        return field_spec_list

    def get_field(self, field_spec: ProcfsStructFieldSpec) -> ProcfsStructVal:
        return eval(f"self.{field_spec}")

    def set_field(self, field_spec: ProcfsStructFieldSpec, val: ProcfsStructVal):
        exec(f"self.{field_spec} = {val!r}")


def proc_pid_dir(pid: int, tid: int = 0, procfs_root: str = TestdataProcfsRoot) -> str:
    if tid == 0:
        return os.path.join(procfs_root, str(pid))
    else:
        return os.path.join(procfs_root, str(pid), "task", str(tid))
