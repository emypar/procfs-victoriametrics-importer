#! /usr/bin/env python3

# tools for pvmi test support

import dataclasses
import os
import sys
from typing import Callable, Dict, List, Optional, Union

py_tools_dir = os.path.dirname(
    os.path.normpath(os.path.abspath(os.path.dirname(__file__)))
)
sys.path.extend(py_tools_dir)

MetricsFnMap = Dict[Callable, bool]


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


def metrics_delta(prev_metrics: List[Metric], metrics: List[Metric]) -> List[Metric]:
    prev_metrics_name_val = set(
        (m.metric, m.val if m.valfmt is None else f"{m.val:{m.valfmt}}")
        for m in prev_metrics
    )
    delta_metrics = []
    for m in metrics:
        val = m.val if m.valfmt is None else f"{m.val:{m.valfmt}}"
        if (m.metric, val) not in prev_metrics_name_val:
            delta_metrics.append(m)
    return delta_metrics


def register_metrics_fn(
    metrics_fn_map: MetricsFnMap,
    require_history: bool = False,
) -> Callable:
    def wrapper(fn: Callable) -> Callable:
        metrics_fn_map[fn] = require_history
        return fn

    return wrapper
