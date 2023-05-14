#! /usr/bin/env python3

# support for pvmi/proc_interrupts_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import Dict, List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from . import proc_interrupts_metrics
from .common import Metric, metrics_delta, ts_to_go_time

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_INTERRUPTS_METRICS_TEST_CASES_FILE_NAME = "proc_interrupts_metrics_test_cases.json"
PROC_INTERRUPTS_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_interrupts_metrics_delta_test_cases.json"
)
REMOVED_PROC_INTERRUPTS = {
    "REMOVED_IRQ": procfs.Interrupt(Info="non existent", Devices="none", Values=[])
}


@dataclasses.dataclass
class ProcInterruptsMetricsTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevInterrupts: Optional[procfs.Interrupts] = None
    PrevTimestamp: int = 0
    PrevRefreshGroupNum: Optional[Dict[str, int]] = None
    PrevNextRefreshGroupNum: int = 0
    WantInterrupts: Optional[procfs.Interrupts] = None
    Timestamp: int = 0
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0
    WantRefreshGroupNum: Optional[Dict[str, int]] = None
    WantNextRefreshGroupNum: int = 0


def load_pirqtc(
    name: str = "full",
    proc_interrupts: Optional[procfs.Interrupts] = None,
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
) -> ProcInterruptsMetricsTestCase:
    if proc_interrupts is None:
        proc_interrupts = procfs.load_interrupts(procfs_root=procfs_root)
    for interrupt in proc_interrupts.values():
        ts = interrupt._ts
        break
    timestamp = ts_to_go_time(ts)
    tc = ProcInterruptsMetricsTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantInterrupts=proc_interrupts,
        Timestamp=timestamp,
        WantMetrics=proc_interrupts_metrics.proc_interrupts_metrics(proc_interrupts),
        FullMetricsFactor=full_metrics_factor,
    )
    if tc.FullMetricsFactor > 1:
        tc.WantRefreshGroupNum = {}
        tc.WantNextRefreshGroupNum = tc.PrevNextRefreshGroupNum
        for irq in sorted(tc.WantInterrupts):
            tc.WantRefreshGroupNum[irq] = tc.WantNextRefreshGroupNum
            tc.WantNextRefreshGroupNum += 1
            if tc.WantNextRefreshGroupNum >= tc.FullMetricsFactor:
                tc.WantNextRefreshGroupNum = 0
    return tc


def make_full_refresh_pirqtc(
    name: str = "full-refresh",
    proc_interrupts: Optional[procfs.Interrupts] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcInterruptsMetricsTestCase]:
    pirqtc_list = []
    if proc_interrupts is None:
        proc_interrupts = procfs.load_interrupts(procfs_root=procfs_root)
    for interrupt in proc_interrupts.values():
        ts = interrupt._ts
        break
    timestamp = ts_to_go_time(ts)
    irq_list = sorted(proc_interrupts)
    want_next_refresh_group_num = len(proc_interrupts)
    for full_metrics_factor in range(2, len(proc_interrupts) + 2):
        refresh_group_num = {
            irq: i % full_metrics_factor for (i, irq) in enumerate(irq_list)
        }
        want_next_refresh_group_num = len(refresh_group_num) % full_metrics_factor
        for refresh_cycle_num in range(full_metrics_factor):
            refresh_proc_interrupts = {
                irq: proc_interrupts[irq]
                for irq in proc_interrupts
                if refresh_group_num[irq] == refresh_cycle_num
            }
            want_metrics = proc_interrupts_metrics.proc_interrupts_metrics(
                refresh_proc_interrupts
            )
            tc = ProcInterruptsMetricsTestCase(
                Name=f'{name}:{"+".join(sorted(refresh_proc_interrupts))}',
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevInterrupts=proc_interrupts,
                PrevRefreshGroupNum=refresh_group_num,
                PrevNextRefreshGroupNum=want_next_refresh_group_num,
                WantInterrupts=proc_interrupts,
                Timestamp=timestamp,
                WantMetrics=want_metrics,
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
                WantRefreshGroupNum=refresh_group_num,
                WantNextRefreshGroupNum=want_next_refresh_group_num,
            )
            pirqtc_list.append(tc)
    return pirqtc_list


def make_removed_irq_pirqtc(
    name: str = "removed-irq",
    proc_interrupts: Optional[procfs.Interrupts] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> ProcInterruptsMetricsTestCase:
    if proc_interrupts is None:
        proc_interrupts = procfs.load_interrupts(procfs_root=procfs_root)
    for interrupt in proc_interrupts.values():
        ts = interrupt._ts
        break
    timestamp = ts_to_go_time(ts)
    prev_proc_interrupts = deepcopy(proc_interrupts)
    prev_proc_interrupts.update(REMOVED_PROC_INTERRUPTS)
    prev_irq_list = sorted(prev_proc_interrupts)
    # Ensure that no irq will have a full refresh cycle:
    full_metrics_factor = 2 * len(prev_irq_list) + 1
    refresh_cycle_num = full_metrics_factor - 1
    prev_refresh_group_num = {
        irq: i % full_metrics_factor for (i, irq) in enumerate(prev_irq_list)
    }
    want_next_refresh_group_num = len(prev_refresh_group_num) % full_metrics_factor
    want_refresh_group_num = deepcopy(prev_refresh_group_num)
    for irq in REMOVED_PROC_INTERRUPTS:
        del want_refresh_group_num[irq]
    tc = ProcInterruptsMetricsTestCase(
        Name=f'{name}:{"+".join(sorted(REMOVED_PROC_INTERRUPTS))}',
        ProcfsRoot=rel_path_to_file(procfs_root),
        PrevInterrupts=prev_proc_interrupts,
        PrevRefreshGroupNum=prev_refresh_group_num,
        PrevNextRefreshGroupNum=want_next_refresh_group_num,
        WantInterrupts=proc_interrupts,
        Timestamp=timestamp,
        WantMetrics=proc_interrupts_metrics.proc_interrupts_metrics(
            REMOVED_PROC_INTERRUPTS,
            ts=ts,
            _clear_pseudo_only=True,
        ),
        FullMetricsFactor=full_metrics_factor,
        RefreshCycleNum=refresh_cycle_num,
        WantRefreshGroupNum=want_refresh_group_num,
        WantNextRefreshGroupNum=want_next_refresh_group_num,
    )
    return tc


def make_delta_pirqtc(
    name: str = "delta",
    proc_interrupts: Optional[procfs.Interrupts] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcInterruptsMetricsTestCase]:
    pirqtc_list = []
    if proc_interrupts is None:
        proc_interrupts = procfs.load_interrupts(procfs_root=procfs_root)
    for interrupt in proc_interrupts.values():
        ts = interrupt._ts
        break
    timestamp = ts_to_go_time(ts)
    irq_list = sorted(proc_interrupts)
    # Ensure that no irq will have a full refresh cycle:
    full_metrics_factor = 2 * len(irq_list) + 1
    refresh_cycle_num = full_metrics_factor - 1
    refresh_group_num = {
        irq: i % full_metrics_factor for (i, irq) in enumerate(irq_list)
    }
    next_refresh_group_num = len(irq_list) % full_metrics_factor
    for irq, interrupt in proc_interrupts.items():
        for cpu, value in enumerate(interrupt.Values):
            prev_value = "0" if value != "0" else "1"
            prev_proc_interrupts = deepcopy(proc_interrupts)
            prev_proc_interrupts[irq].Values[cpu] = prev_value
            want_metrics = metrics_delta(
                proc_interrupts_metrics.proc_interrupts_metrics(prev_proc_interrupts),
                proc_interrupts_metrics.proc_interrupts_metrics(proc_interrupts),
            )
            tc = ProcInterruptsMetricsTestCase(
                Name=f"{name}:cpu={cpu},irq={irq}:{prev_value}->{value}",
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevInterrupts=prev_proc_interrupts,
                PrevRefreshGroupNum=refresh_group_num,
                PrevNextRefreshGroupNum=next_refresh_group_num,
                WantInterrupts=proc_interrupts,
                Timestamp=timestamp,
                WantMetrics=want_metrics,
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
                WantRefreshGroupNum=refresh_group_num,
                WantNextRefreshGroupNum=next_refresh_group_num,
            )
            pirqtc_list.append(tc)
    return pirqtc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    pirqtc_list = []
    proc_interrupts = procfs.load_interrupts(procfs_root=procfs_root)
    for full_metrics_factor in [0, 1, len(proc_interrupts) + 1]:
        pirqtc_list.append(
            load_pirqtc(
                proc_interrupts=proc_interrupts,
                full_metrics_factor=full_metrics_factor,
            )
        )
    pirqtc_list.extend(make_full_refresh_pirqtc(proc_interrupts=proc_interrupts))
    pirqtc_list.append(make_removed_irq_pirqtc(proc_interrupts=proc_interrupts))
    save_to_json_file(
        [pirqtc.to_json_compat() for pirqtc in pirqtc_list],
        os.path.join(test_case_dir, PROC_INTERRUPTS_METRICS_TEST_CASES_FILE_NAME),
    )
    pirqtc_list = make_delta_pirqtc(proc_interrupts=proc_interrupts)
    save_to_json_file(
        [pirqtc.to_json_compat() for pirqtc in pirqtc_list],
        os.path.join(test_case_dir, PROC_INTERRUPTS_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
