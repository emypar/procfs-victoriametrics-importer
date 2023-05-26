# /usr/bin/env python3

import dataclasses
import json
import os
import sys
from typing import Any, List

from metrics_common_test import PVMI_TOP_DIR

pvmi_go_pkg_dir = os.path.join(PVMI_TOP_DIR, "pvmi")

StructBaseVal = Any
StructBaseFieldSpec = str


def get_field_spec_list(obj: Any) -> List[StructBaseFieldSpec]:
    field_spec_list = []
    if isinstance(obj, list):
        for i, val in enumerate(obj):
            field_spec_list.extend(f"[{i}]{f}" for f in get_field_spec_list(val))
    elif isinstance(obj, dict):
        for k, val in obj.items():
            field_spec_list.extend(f"[{k!r}]{f}" for f in get_field_spec_list(val))
    elif hasattr(obj, "field_list"):
        for field in obj.field_list():
            field_spec_list.extend(
                f".{field}{f}"
                for f in get_field_spec_list(getattr(obj, field))
                if not field.startswith("_")
            )
    else:
        field_spec_list.append("")
    return field_spec_list


def to_json_compat(obj: Any, ignore_none: bool = False) -> Any:
    if hasattr(obj, "_to_json_compat"):
        return obj._to_json_compat(ignore_none=ignore_none)
    if isinstance(obj, bytes):
        return list(map(int, obj))
    if isinstance(obj, (list, tuple)):
        return list(to_json_compat(o, ignore_none=ignore_none) for o in obj)
    if isinstance(obj, set):
        return {str(o): True for o in obj if o is not None or not ignore_none}
    if isinstance(obj, dict):
        return {
            str(k): to_json_compat(v, ignore_none=ignore_none)
            for k, v in obj.items()
            if v is not None or not ignore_none
        }
    if hasattr(obj, "field_list"):
        return {
            field: to_json_compat(getattr(obj, field), ignore_none)
            for field in obj.field_list()
            if getattr(obj, field) is not None or not ignore_none
        }
    return obj


@dataclasses.dataclass
class StructBase:
    def field_list(self):
        return [
            field for field in self.__dataclass_fields__ if not field.startswith("_")
        ]

    def get_field_spec_list(self) -> List[StructBaseFieldSpec]:
        return get_field_spec_list(self)

    def get_field(self, field_spec: StructBaseFieldSpec) -> StructBaseVal:
        return eval(f"self{field_spec}")

    def set_field(self, field_spec: StructBaseFieldSpec, val: StructBaseVal):
        exec(f"self{field_spec} = {val!r}")

    def to_json_compat(self, ignore_none: bool = False):
        return to_json_compat(self, ignore_none=ignore_none)


def ts_to_prometheus_ts(ts: float) -> int:
    return int(ts * 1000.0)  # UTC in milliseconds


def sanitize_label_value(v: str) -> str:
    return v.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def rel_path_to_file(src_path: str, ref_path: str = pvmi_go_pkg_dir) -> str:
    src_path = os.path.normpath(os.path.abspath(src_path))
    ref_path = os.path.normpath(os.path.abspath(ref_path))
    if os.path.isfile(ref_path):
        ref_path = os.path.dirname(ref_path)
    return os.path.relpath(src_path, ref_path)


def save_to_json_file(obj: Any, f_name: str):
    print(f"{f_name!r} ...", file=sys.stderr, end="")
    os.makedirs(os.path.dirname(f_name), exist_ok=True)
    with open(f_name, "wt") as f:
        json.dump(obj, f, indent=2)
        f.write("\n")
    print(" done", file=sys.stderr)
