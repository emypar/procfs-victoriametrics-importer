# /usr/bin/env python3

import dataclasses
import json
import os
import sys
from typing import Any

from metrics_common_test import PVMI_TOP_DIR

pvmi_pkg_dir = os.path.join(PVMI_TOP_DIR, "pvmi")


@dataclasses.dataclass
class StructBase:
    def field_list(self):
        return [
            field for field in self.__dataclass_fields__ if not field.startswith("_")
        ]

    def to_json_compat(self, ignore_none: bool = False):
        json_compat = {}
        for field in self.field_list():
            val = getattr(self, field)
            if isinstance(val, bytes):
                val = list(map(int, val))
            elif isinstance(val, (list, tuple)):
                val = [
                    v.to_json_compat(ignore_none=ignore_none)
                    if hasattr(v, "to_json_compat")
                    else v
                    for v in val
                    if v is not None or not ignore_none
                ]
            elif isinstance(val, set):
                val = {str(v): True for v in val if v is not None or not ignore_none}
            elif isinstance(val, dict):
                val = {
                    str(k): v.to_json_compat(ignore_none=ignore_none)
                    if hasattr(v, "to_json_compat")
                    else v
                    for (k, v) in val.items()
                }
            elif hasattr(val, "to_json_compat"):
                val = val.to_json_compat(ignore_none=ignore_none)
            if val is not None or not ignore_none:
                json_compat[field] = val
        return json_compat


def ts_to_prometheus_ts(ts: float) -> int:
    return int(ts * 1000.0)  # UTC in milliseconds


def sanitize_label_value(v: str) -> str:
    return v.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def rel_path_to_file(src_path: str, ref_path: str = pvmi_pkg_dir) -> str:
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
    print(" done", file=sys.stderr)
