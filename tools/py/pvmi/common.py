#! /usr/bin/env python3

# tools for pvmi test support

import dataclasses
import os
import sys
import time
from datetime import datetime, timezone
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


def ts_to_prometheus_ts(ts: float) -> int:
    return int(ts * 1000.0)


def prometheus_ts_to_ts(prom_ts: int) -> float:
    return float(prom_ts) / 1000.0


def ts_to_go_time(ts: Optional[float] = 0) -> str:
    return datetime.fromtimestamp(
        ts if ts is not None else time.time(), timezone.utc
    ).isoformat()


def go_time_to_ts(timestamp: str) -> float:
    ts = datetime.fromisoformat(timestamp)
    return ts.timestamp()


def go_time_to_prometheus_ts(timestamp: str) -> int:
    ts = datetime.fromisoformat(timestamp)
    return ts_to_prometheus_ts(ts.timestamp())


def sanitize_label_value(v: str) -> str:
    return v.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")


def metrics_delta(
    prev_metrics: List[Metric],
    metrics: List[Metric],
) -> List[Metric]:
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
