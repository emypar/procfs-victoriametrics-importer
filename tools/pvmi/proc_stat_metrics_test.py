#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from . import proc_stat_metrics
from .common import Metric, metrics_delta

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_STAT_METRICS_TEST_CASES_FILE_NAME = "proc_stat_metrics_test_cases.json"
PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_stat_metrics_delta_test_cases.json"
PROC_STAT_METRICS_DELTA_INTERVAL = 1000  # milliseconds (Prometheus)
ALWAYS_IGNORE_FIELDS = {"IRQ", "SoftIRQ"}


@dataclasses.dataclass
class ProcStatMetricsTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevStat: Optional[procfs.Stat] = None
    PrevTimestamp: int = 0
    WantStat: Optional[procfs.Stat] = None
    Timestamp: int = 0
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0


def load_psmtc(
    name: str = "",
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
    refresh_cycle_num: int = 0,
) -> ProcStatMetricsTestCase:
    proc_stat = procfs.load_stat(procfs_root=procfs_root)
    return ProcStatMetricsTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantStat=proc_stat,
        Timestamp=proc_stat._ts,
        WantMetrics=proc_stat_metrics.generate_all_metrics(proc_stat),
        FullMetricsFactor=full_metrics_factor,
        RefreshCycleNum=refresh_cycle_num,
    )


def make_refresh_psmtc(
    name: str = "refresh-cycle",
    procfs_root: str = TestdataProcfsRoot,
    proc_stat: Optional[procfs.Stat] = None,
) -> List[ProcStatMetricsTestCase]:
    psmtc_list = []
    if proc_stat is None:
        proc_stat = procfs.load_stat(procfs_root=procfs_root)
    # See pvmi/proc_stat_metrics.go refreshCycleNum for concepts:
    num_refresh_cycles = 2 + len(proc_stat.CPU)
    full_metrics_factor = num_refresh_cycles + 1
    ts = proc_stat._ts
    prev_proc_stat = deepcopy(proc_stat)
    prev_ts = ts - PROC_STAT_METRICS_DELTA_INTERVAL
    prev_proc_stat._ts = prev_ts
    full_metrics = proc_stat_metrics.generate_all_metrics(
        proc_stat, prev_proc_stat=prev_proc_stat, _full_metrics=True
    )
    for refresh_cycle_num in range(num_refresh_cycles):
        psmtc = ProcStatMetricsTestCase(
            Name=name,
            ProcfsRoot=rel_path_to_file(procfs_root),
            PrevStat=proc_stat,
            PrevTimestamp=prev_ts,
            WantStat=proc_stat,
            Timestamp=ts,
            FullMetricsFactor=full_metrics_factor,
            RefreshCycleNum=refresh_cycle_num,
        )
        if refresh_cycle_num == 0:
            # Keep only metrics that have no cpu=".*" labels:
            psmtc.WantMetrics = [
                metric for metric in full_metrics if 'cpu="' not in metric.metric
            ]
        elif refresh_cycle_num == 1:
            # Keep only cpu="all":
            psmtc.WantMetrics = [
                metric for metric in full_metrics if 'cpu="all"' in metric.metric
            ]
        else:
            # Keep only cpu="{refresh_cycle_num - 2}":
            psmtc.WantMetrics = [
                metric
                for metric in full_metrics
                if f'cpu="{refresh_cycle_num - 2}"' in metric.metric
            ]
        psmtc_list.append(psmtc)
    return psmtc_list


def make_delta_psmtc(
    name: str = "delta",
    procfs_root: str = TestdataProcfsRoot,
    proc_stat: Optional[procfs.Stat] = None,
) -> List[ProcStatMetricsTestCase]:
    psmtc_list = []
    if proc_stat is None:
        proc_stat = procfs.load_stat(procfs_root=procfs_root)
    # See pvmi/proc_stat_metrics.go refreshCycleNum for concepts.
    num_refresh_cycles = 2 + len(proc_stat.CPU)
    full_metrics_factor = num_refresh_cycles + 1
    ts = proc_stat._ts
    prev_ts = ts - PROC_STAT_METRICS_DELTA_INTERVAL
    for field_spec in proc_stat.get_field_spec_list():
        val = proc_stat.get_field(field_spec)
        if not isinstance(val, (int, float)) or val == 0:
            continue
        prev_proc_stat = deepcopy(proc_stat)
        prev_proc_stat._ts = prev_ts
        prev_val = val - min(val, 1)
        prev_proc_stat.set_field(field_spec, prev_val)
        prev_full_metrics = proc_stat_metrics.generate_all_metrics(prev_proc_stat)
        # Choose a refresh cycle such that this field is not due for change:
        if field_spec.startswith("CPU"):
            refresh_cycle_num = 0
            full_metrics = proc_stat_metrics.generate_all_metrics(
                proc_stat, prev_proc_stat=prev_proc_stat, _full_metrics=False
            )
            delta_metrics = metrics_delta(prev_full_metrics, full_metrics)
            want_metrics = delta_metrics + [
                metric for metric in full_metrics if 'cpu="' not in metric.metric
            ]
        else:
            refresh_cycle_num = 1
            full_metrics = proc_stat_metrics.generate_all_metrics(
                proc_stat, prev_proc_stat=prev_proc_stat, _full_metrics=True
            )
            delta_metrics = metrics_delta(prev_full_metrics, full_metrics)
            want_metrics = [
                metric for metric in delta_metrics if 'cpu="' not in metric.metric
            ] + [metric for metric in full_metrics if 'cpu="all"' in metric.metric]
        psmtc_list.append(
            ProcStatMetricsTestCase(
                Name=f"{name}:{field_spec}:{prev_val}->{val}",
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevStat=prev_proc_stat,
                PrevTimestamp=prev_ts,
                WantStat=proc_stat,
                Timestamp=ts,
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
                WantMetrics=want_metrics,
            )
        )
    return psmtc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    psmtc_list = []
    psmtc_list.append(load_psmtc(name="full"))
    psmtc_list.extend(make_refresh_psmtc(name="refresh-cycle"))
    save_to_json_file(
        [psmtc.to_json_compat() for psmtc in psmtc_list],
        os.path.join(test_case_dir, PROC_STAT_METRICS_TEST_CASES_FILE_NAME),
    )

    psmtc_list = make_delta_psmtc(name="delta")
    save_to_json_file(
        [psmtc.to_json_compat() for psmtc in psmtc_list],
        os.path.join(test_case_dir, PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
