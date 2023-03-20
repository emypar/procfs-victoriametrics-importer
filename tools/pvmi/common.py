#! /usr/bin/env python3

# tools for pvmi test support

import dataclasses
import os
import sys
from typing import Callable, Dict, List, Optional, Tuple, Union

from procfs import ProcfsStructFieldSpec

tools_dir = os.path.dirname(
    os.path.normpath(os.path.abspath(os.path.dirname(__file__)))
)
sys.path.extend(tools_dir)


FieldToMetricsFnMap = Dict[ProcfsStructFieldSpec, List[Tuple[Callable, bool]]]


@dataclasses.dataclass
class Metric:
    metric: str = ""
    val: Union[int, float] = 0
    ts: int = 0
    valfmt: Optional[str] = None

    def asstr(self, valfmt: Optional[str] = None) -> str:
        if valfmt is None:
            valfmt = self.valfmt
        if valfmt is not None:
            valstr = f"{self.val:{valfmt}}"
        else:
            valstr = str(self.val)
        return " ".join([self.metric, valstr, str(self.ts)])

    def to_json_compat(self, ignore_none: bool = False):
        return self.asstr()

    def name(self):
        i = self.metric.find("{")
        return self.metric[:i] if i >= 0 else self.metric


def register_metrics_fn(
    metrics_fn_map: FieldToMetricsFnMap,
    field_spec_or_specs: Union[
        ProcfsStructFieldSpec, List[ProcfsStructFieldSpec], None
    ] = None,
    require_history: bool = False,
) -> Callable:
    """Decorator: register a function as generating metric(s) for the given
    fields.

    Args:
        metrics_fn_map: the map field spec -> (function, require_history)
    """
    field_spec_list = (
        field_spec_or_specs
        if isinstance(field_spec_or_specs, list)
        else [field_spec_or_specs]
    )

    field_specs_added_for_this_fn = set()

    def add_field(field_spec, fn):
        if field_spec in field_specs_added_for_this_fn:
            return
        if field_spec not in metrics_fn_map:
            metrics_fn_map[field_spec] = []
        metrics_fn_map[field_spec].append((fn, require_history))
        field_specs_added_for_this_fn.add(field_spec)

    def wrapper(fn: Callable) -> Callable:
        for field_spec in field_spec_list:
            add_field(field_spec, fn)
            if isinstance(field_spec, tuple):
                # This is a list type of field which was specified w/ an index,
                # add the field name proper too:
                add_field(field_spec[0], fn)
        return fn

    return wrapper


def get_metrics_fn_list(
    metrics_fn_map_or_module: FieldToMetricsFnMap,
) -> List[Tuple[Callable, bool]]:
    if not isinstance(metrics_fn_map_or_module, dict) and hasattr(
        metrics_fn_map_or_module, "metrics_fn_map"
    ):
        # Maybe it's a module:
        metrics_fn_map = getattr(metrics_fn_map_or_module, "metrics_fn_map")
    else:
        metrics_fn_map = metrics_fn_map_or_module
    full_fn_list = set()
    for fn_list in metrics_fn_map.values():
        full_fn_list.update(fn_list)
    return list(full_fn_list)
