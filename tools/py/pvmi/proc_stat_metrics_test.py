#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import Dict, List, Optional

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from . import proc_stat_metrics
from .common import Metric, metrics_delta, ts_to_go_time

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_STAT_METRICS_TEST_CASES_FILE_NAME = "proc_stat_metrics_test_cases.json"
PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_stat_metrics_delta_test_cases.json"
PROC_STAT_METRICS_DELTA_INTERVAL_SECONDS = 1


@dataclasses.dataclass
class PCpu(StructBase):
    User: float = 0
    Nice: float = 0
    System: float = 0
    Idle: float = 0
    Iowait: float = 0
    IRQ: float = 0
    SoftIRQ: float = 0
    Steal: float = 0
    Guest: float = 0
    GuestNice: float = 0


@dataclasses.dataclass
class ProcStatMetricsTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevStat: List[procfs.Stat2] = dataclasses.field(
        default_factory=lambda: [None, None]
    )
    PrevCrtIndex: int = 0
    PrevPCpu: Optional[Dict[int, PCpu]] = None
    PrevTimestamp: Optional[str] = None
    WantStat: List[procfs.Stat2] = dataclasses.field(
        default_factory=lambda: [None, None]
    )
    WantCrtIndex: int = 0
    WantPCpu: Optional[Dict[int, PCpu]] = None
    Timestamp: str = dataclasses.field(default_factory=ts_to_go_time)
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0


def make_proc_stat_pcpu(
    proc_stat: procfs.Stat2,
    prev_proc_stat: procfs.Stat2,
    ts: Optional[float] = None,
    prev_ts: Optional[float] = None,
) -> Dict[int, PCpu]:
    pcpu = {}
    if ts is None:
        ts = proc_stat._ts
    if prev_ts is None:
        prev_ts = prev_proc_stat._ts
    delta_seconds = ts - prev_ts
    for i, cpu_stat in enumerate(proc_stat.CPU):
        prev_cpu_stat = prev_proc_stat.CPU[i]
        cpu = cpu_stat.Cpu
        pcpu[cpu] = PCpu()
        prev_cpu_stat = prev_proc_stat.CPU[i]
        for field in [
            "User",
            "Nice",
            "System",
            "Idle",
            "Iowait",
            "IRQ",
            "SoftIRQ",
            "Steal",
            "Guest",
            "GuestNice",
        ]:
            setattr(
                pcpu[cpu],
                field,
                (cpu_stat.get_field(field) - prev_cpu_stat.get_field(field))
                * TestClktckSec
                / delta_seconds
                * 100.0,
            )
    return pcpu


def load_psmtc(
    name: str = "full",
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
    refresh_cycle_num: int = 0,
) -> ProcStatMetricsTestCase:
    proc_stat = procfs.load_stat2(procfs_root=procfs_root)
    psmtc = ProcStatMetricsTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        PrevCrtIndex=1,
        Timestamp=ts_to_go_time(proc_stat._ts),
        WantCrtIndex=0,
        WantMetrics=proc_stat_metrics.proc_stat_metrics(proc_stat),
        FullMetricsFactor=full_metrics_factor,
        RefreshCycleNum=refresh_cycle_num,
    )
    psmtc.WantStat[psmtc.WantCrtIndex] = proc_stat
    return psmtc


def make_refresh_psmtc(
    name: str = "refresh-cycle",
    procfs_root: str = TestdataProcfsRoot,
    proc_stat: Optional[procfs.Stat2] = None,
) -> List[ProcStatMetricsTestCase]:
    psmtc_list = []
    if proc_stat is None:
        proc_stat = procfs.load_stat2(procfs_root=procfs_root)
    ts = proc_stat._ts
    prev_proc_stat = deepcopy(proc_stat)
    prev_ts = ts - PROC_STAT_METRICS_DELTA_INTERVAL_SECONDS
    prev_proc_stat._ts = prev_ts
    full_metrics = proc_stat_metrics.proc_stat_metrics(
        proc_stat, prev_proc_stat=prev_proc_stat
    )
    pcpu = make_proc_stat_pcpu(proc_stat, prev_proc_stat)
    for prev_pcpu in [None, deepcopy(pcpu)]:
        # See pvmi/proc_stat_metrics.go refreshCycleNum for concepts:
        for full_metrics_factor in range(1, 2 + len(proc_stat.CPU)):
            for refresh_cycle_num in range(full_metrics_factor):
                stats_group_num = 0
                psmtc = ProcStatMetricsTestCase(
                    Name=f"{name}:prevPcpu={prev_pcpu is not None}",
                    ProcfsRoot=rel_path_to_file(procfs_root),
                    PrevStat=[prev_proc_stat, None],
                    PrevCrtIndex=0,
                    PrevPCpu=prev_pcpu,
                    PrevTimestamp=ts_to_go_time(prev_ts),
                    WantStat=[prev_proc_stat, proc_stat],
                    WantCrtIndex=1,
                    WantPCpu=pcpu,
                    Timestamp=ts_to_go_time(ts),
                    FullMetricsFactor=full_metrics_factor,
                    RefreshCycleNum=refresh_cycle_num,
                )
                want_metrics = []
                if stats_group_num == refresh_cycle_num:
                    # Keep only metrics that have no cpu=".*" labels:
                    want_metrics.extend(
                        metric
                        for metric in full_metrics
                        if 'cpu="' not in metric.metric
                    )
                stats_group_num = (stats_group_num + 1) % full_metrics_factor
                for cpu_stat in proc_stat.CPU:
                    if stats_group_num == refresh_cycle_num:
                        cpu = "all" if cpu_stat.Cpu == -1 else str(cpu_stat.Cpu)
                        # Keep only cpu="<cpu>"; do not add pct metrics if prev_pcpu is
                        # None, they are handled in bulk at the end:
                        want_metrics.extend(
                            metric
                            for metric in full_metrics
                            if f'cpu="{cpu}"' in metric.metric
                            and (
                                prev_pcpu is not None
                                or metric.name() != "proc_stat_cpu_time_pct"
                            )
                        )
                    stats_group_num = (stats_group_num + 1) % full_metrics_factor
                # If prev_pcpu is None, add all pct metrics:
                if prev_pcpu is None:
                    want_metrics.extend(
                        metric
                        for metric in full_metrics
                        if metric.name() == "proc_stat_cpu_time_pct"
                    )
                psmtc.WantMetrics = want_metrics
                psmtc_list.append(psmtc)
    return psmtc_list


def make_delta_psmtc(
    name: str = "delta",
    procfs_root: str = TestdataProcfsRoot,
    proc_stat: Optional[procfs.Stat2] = None,
) -> List[ProcStatMetricsTestCase]:
    psmtc_list = []
    if proc_stat is None:
        proc_stat = procfs.load_stat2(procfs_root=procfs_root)
    # See pvmi/proc_stat_metrics.go refreshCycleNum for concepts. Choose a
    # refresh cycle such that no full refresh will occur:
    full_metrics_factor = 2 + len(proc_stat.CPU)
    refresh_cycle_num = full_metrics_factor - 1
    ts = proc_stat._ts
    prev_ts = ts - PROC_STAT_METRICS_DELTA_INTERVAL_SECONDS
    prev_prev_ts = prev_ts - PROC_STAT_METRICS_DELTA_INTERVAL_SECONDS
    for field_spec in proc_stat.get_field_spec_list():
        if field_spec.endswith("].Cpu"):
            continue
        val = proc_stat.get_field(field_spec)
        if not isinstance(val, (int, float)) or val == 0:
            continue
        prev_proc_stat = deepcopy(proc_stat)
        prev_proc_stat._ts = prev_ts
        prev_val = val - min(val, 1)
        prev_proc_stat.set_field(field_spec, prev_val)
        full_metrics = proc_stat_metrics.proc_stat_metrics(
            proc_stat,
            prev_proc_stat=prev_proc_stat,
        )
        prev_full_metrics = proc_stat_metrics.proc_stat_metrics(
            prev_proc_stat, prev_proc_stat=prev_proc_stat, prev_ts=prev_prev_ts
        )
        pcpu = make_proc_stat_pcpu(proc_stat, prev_proc_stat)
        prev_pcpu = make_proc_stat_pcpu(
            prev_proc_stat, prev_proc_stat, prev_ts=prev_prev_ts
        )
        psmtc_list.append(
            ProcStatMetricsTestCase(
                Name=f"{name}:{field_spec}:{prev_val}->{val}",
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevStat=[prev_proc_stat, prev_proc_stat],
                PrevCrtIndex=0,
                PrevPCpu=prev_pcpu,
                PrevTimestamp=ts_to_go_time(prev_ts),
                WantStat=[prev_proc_stat, proc_stat],
                WantCrtIndex=1,
                WantPCpu=pcpu,
                Timestamp=ts_to_go_time(ts),
                WantMetrics=metrics_delta(prev_full_metrics, full_metrics),
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
            )
        )
    return psmtc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    psmtc_list = []
    psmtc_list.append(load_psmtc(procfs_root=procfs_root))
    psmtc_list.extend(make_refresh_psmtc(procfs_root=procfs_root))
    save_to_json_file(
        [psmtc.to_json_compat() for psmtc in psmtc_list],
        os.path.join(test_case_dir, PROC_STAT_METRICS_TEST_CASES_FILE_NAME),
    )
    psmtc_list = make_delta_psmtc(procfs_root=procfs_root)
    save_to_json_file(
        [psmtc.to_json_compat() for psmtc in psmtc_list],
        os.path.join(test_case_dir, PROC_STAT_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
