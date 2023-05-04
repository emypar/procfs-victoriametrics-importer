#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import List, Optional, Tuple, Union

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from .common import Metric, metrics_delta, ts_to_prometheus_ts
from .pid_list import get_pid_tid_list
from .proc_pid_metrics import (
    PidMetricsCacheEntry,
    PmceStatFieldNames,
    generate_pmce_full_metrics,
    load_pmce,
    update_pmce,
)

# The following should match pvmi/proc_pid_metrics_test.go:
PID_METRICS_TEST_CASE_LOAD_FILE_NAME = "proc_pid_metrics_test_case_load.json"
PID_METRICS_TEST_CASES_FILE_NAME = "proc_pid_metrics_test_cases.json"
PID_METRICS_DELTA_TEST_CASES_FILE_NAME = "proc_pid_metrics_delta_test_cases.json"
ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_all_pid_metrics_delta_test_cases.json"
)

PID_SCAN_INTERVAL_SECONDS = 1
PREV_PROC_PID_CGROUP_DELTA = b"999999:controller_1,controller_2:path\n"
PREV_PROC_PID_CMDLINE_DELTA = b"__no-such-command__\0"
GUARANTEED_ACTIVE_THRESHOLD = 0
# Next use more cpu ticks than exist in the scan interval:
GUARANTEED_INACTIVE_THRESHOLD = 2 * int(PID_SCAN_INTERVAL_SECONDS / TestClktckSec + 1)

# List of (stat_field, field_spec) that should be ignored for delta test cases:
STAT_FIELD_SPEC_IGNORE_FOR_DELTA = {
    ("ProcStat", "PID"),
    ("ProcStat", "Starttime"),
    ("ProcStatus", "PID"),
}


def procfs_struct_val_variant(val: procfs.ProcfsStructVal) -> procfs.ProcfsStructVal:
    """Return a variant for a field value, useful for forcing a delta"""
    if isinstance(val, list):
        return list(map(procfs_struct_val_variant, val))
    elif isinstance(val, (int, float)):
        return 0 if val != 0 else 1
    elif isinstance(val, str):
        return "" if val != "" else "_"
    else:
        return val


def alter_procfs_struct_field(
    struct: procfs.ProcfsStructType,
    field_spec: procfs.ProcfsStructFieldSpec,
):
    struct.set_field(
        field_spec, procfs_struct_val_variant(struct.get_field(field_spec))
    )


@dataclasses.dataclass
class PidMetricsTestCase(StructBase):
    Name: str = ""
    Pid: int = 0
    Tid: int = 0
    ProcfsRoot: str = ""
    PrevPmce: Optional[PidMetricsCacheEntry] = None
    WantPmce: Optional[PidMetricsCacheEntry] = None
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    ActiveThreshold: int = 0


def load_pmtc(
    pid: int, tid: int = 0, name: str = "full", procfs_root: str = TestdataProcfsRoot
) -> PidMetricsTestCase:
    """Return a PidMetricsTestCase for a newly discoverd process/thread"""
    pmce = load_pmce(pid, tid=tid, procfs_root=procfs_root)
    metrics = generate_pmce_full_metrics(pmce)
    return PidMetricsTestCase(
        Name=name,
        Pid=pid,
        Tid=tid,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantPmce=pmce,
        WantMetrics=metrics,
    )


def update_pmtc_metrics(
    pmtc: PidMetricsTestCase,
    metric_or_metrics: Union[Metric, List[Metric], None] = None,
):
    if isinstance(metric_or_metrics, list):
        pmtc.WantMetrics.extend(metric_or_metrics)
    elif metric_or_metrics is not None:
        pmtc.WantMetrics.append(metric_or_metrics)


def make_no_change_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "no-change",
    fullMetricsFactor: Optional[int] = 15,
    refreshCycleNum: Optional[int] = None,
    activeThreshold: Optional[int] = 0,
) -> PidMetricsTestCase:
    """Create a new testcase with no changes from the previous scan"""
    want_pmce = deepcopy(pmtc.WantPmce)
    if want_pmce.PassNum <= 1:
        want_pmce.PassNum = 2
    prev_pmce = deepcopy(want_pmce)
    prev_pmce.Timestamp -= ts_to_prometheus_ts(PID_SCAN_INTERVAL_SECONDS)
    prev_pmce.PassNum -= 1
    if fullMetricsFactor is None:
        fullMetricsFactor = pmtc.FullMetricsFactor
    if refreshCycleNum is None:
        refreshCycleNum = fullMetricsFactor - 1 if fullMetricsFactor > 1 else 0
    prev_pmce.RefreshCycleNum = refreshCycleNum
    want_pmce.RefreshCycleNum = (
        refreshCycleNum + 1 if refreshCycleNum < fullMetricsFactor - 1 else 0
    )
    update_pmce(
        want_pmce,
        prev_pmce=prev_pmce,
    )
    if refreshCycleNum == 0:
        want_metrics = generate_pmce_full_metrics(want_pmce, prev_pmce=prev_pmce)
    else:
        want_metrics = metrics_delta(
            generate_pmce_full_metrics(prev_pmce),
            generate_pmce_full_metrics(want_pmce, prev_pmce=prev_pmce),
        )
    return PidMetricsTestCase(
        Name=name if name is not None else pmtc.Name,
        Pid=pmtc.Pid,
        Tid=pmtc.Tid,
        ProcfsRoot=pmtc.ProcfsRoot,
        PrevPmce=prev_pmce,
        WantPmce=want_pmce,
        WantMetrics=want_metrics,
        FullMetricsFactor=fullMetricsFactor,
        ActiveThreshold=(
            activeThreshold if activeThreshold is not None else pmtc.ActiveThreshold
        ),
    )


def make_active_threshold_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "active-threshold",
) -> List[PidMetricsTestCase]:

    # Generate changes to ProcStat, ProcStatus and ProcIo and use them for 2
    # test cases: above and below active threshold. For the former only
    # ProcStat's ones should show in the generated metrics whereas for the
    # latter all should.

    # The following (stat, stat_field) will generate metrics  regardless of active state:
    always_stat_field_spec_list = [("ProcStat", "RSS")]
    # The following (stat, stat_field) will generate metrics for active processes:
    active_only_stat_field_spec_list = [
        ("ProcStat", "RSS"),
        ("ProcStatus", "VmSize"),
        ("ProcIo", "RChar"),
    ]

    pmtc_list = []
    for active_threshold, stat_field_spec_list in [
        (GUARANTEED_ACTIVE_THRESHOLD, active_only_stat_field_spec_list),
        (GUARANTEED_INACTIVE_THRESHOLD, always_stat_field_spec_list),
    ]:
        new_pmtc = make_no_change_pmtc(
            pmtc,
            name=name,
            fullMetricsFactor=15,
            activeThreshold=active_threshold,
        )
        for stat_field, stat_field_spec in stat_field_spec_list:
            prev_stat = getattr(new_pmtc.PrevPmce, stat_field)
            alter_procfs_struct_field(prev_stat, stat_field_spec)
        update_pmce(new_pmtc.PrevPmce)
        update_pmce(new_pmtc.WantPmce)
        new_pmtc.WantMetrics = metrics_delta(
            generate_pmce_full_metrics(new_pmtc.PrevPmce),
            generate_pmce_full_metrics(
                new_pmtc.WantPmce,
                prev_pmce=new_pmtc.PrevPmce,
            ),
        )
        pmtc_list.append(new_pmtc)
    return pmtc_list


def get_stat_field_spec_list(
    pmtc: PidMetricsTestCase,
) -> List[Tuple[str, procfs.ProcfsStructFieldSpec]]:
    """Get the list of (stat_field, field_spec) pairs that can be used for deltas."""

    stat_field_spec_list = []
    for stat_field in PmceStatFieldNames:
        stat = getattr(pmtc.WantPmce, stat_field)
        field_spec_list = (
            stat.get_field_spec_list()
            if hasattr(stat, "get_field_spec_list")
            else [None]
        )
        for field_spec in field_spec_list:
            stat_field_spec = (stat_field, field_spec)
            if stat_field_spec not in STAT_FIELD_SPEC_IGNORE_FOR_DELTA:
                stat_field_spec_list.append(stat_field_spec)
    return stat_field_spec_list


def make_delta_pmtc(
    pmtc: PidMetricsTestCase,
    stat_field: str,
    stat_field_spec: procfs.ProcfsStructFieldSpec,
    name: Optional[str] = None,
    fullMetricsFactor: Optional[int] = 15,
    activeThreshold: Optional[int] = 0,
) -> PidMetricsTestCase:
    """Build a delta test case where just one field of a given stat changes"""

    if name is None:
        name = stat_field
    if stat_field_spec is not None:
        name += f":{stat_field_spec}"

    want_pmce = deepcopy(pmtc.WantPmce)
    if want_pmce.PassNum <= 1:
        want_pmce.PassNum = 2
    prev_pmce = deepcopy(want_pmce)
    prev_pmce.Timestamp -= ts_to_prometheus_ts(PID_SCAN_INTERVAL_SECONDS)
    prev_pmce.PassNum -= 1
    if fullMetricsFactor is None:
        fullMetricsFactor = pmtc.FullMetricsFactor
    refreshCycleNum = pmtc.FullMetricsFactor - 1
    if stat_field == "RawCgroup":
        update_pmce(prev_pmce, rawCgroup=PREV_PROC_PID_CGROUP_DELTA)
        # Force full metrics cycle or else the changes will not be picked up:
        refreshCycleNum = 0  # metrics published only at full metrics
    elif stat_field == "RawCmdline":
        update_pmce(prev_pmce, rawCmdline=PREV_PROC_PID_CMDLINE_DELTA)
        # Force full metrics cycle or else the changes will not be picked up:
        refreshCycleNum = 0  # metrics published only at full metrics
    elif stat_field != "ProcStat" or stat_field_spec not in {"UTime", "STime"}:
        prev_stat = getattr(prev_pmce, stat_field)
        alter_procfs_struct_field(prev_stat, stat_field_spec)
        update_pmce(prev_pmce)
        refreshCycleNum = fullMetricsFactor - 1 if fullMetricsFactor > 1 else 0
    else:
        # Delta for [US]Time require a value > 0.
        stat = getattr(prev_pmce, stat_field)
        field_val = stat.get_field(stat_field_spec)
        if field_val == 0:
            return None
        # Aim for 50% %CPU:
        prev_field_val = max(
            0, int(field_val - PID_SCAN_INTERVAL_SECONDS / (2 * TestClktckSec))
        )
        stat.set_field(stat_field_spec, prev_field_val)
        update_pmce(prev_pmce, prev_pmce=prev_pmce)
        refreshCycleNum = fullMetricsFactor - 1 if fullMetricsFactor > 1 else 0
    prev_pmce.RefreshCycleNum = refreshCycleNum
    want_pmce.RefreshCycleNum = (
        refreshCycleNum + 1 if refreshCycleNum < fullMetricsFactor - 1 else 0
    )
    update_pmce(
        want_pmce,
        prev_pmce=prev_pmce,
    )
    if refreshCycleNum == 0:
        want_metrics = generate_pmce_full_metrics(want_pmce, prev_pmce=prev_pmce)
    else:
        want_metrics = metrics_delta(
            generate_pmce_full_metrics(prev_pmce),
            generate_pmce_full_metrics(want_pmce, prev_pmce=prev_pmce),
        )
    return PidMetricsTestCase(
        Name=name,
        Pid=pmtc.Pid,
        Tid=pmtc.Tid,
        ProcfsRoot=pmtc.ProcfsRoot,
        PrevPmce=prev_pmce,
        WantPmce=want_pmce,
        WantMetrics=want_metrics,
        FullMetricsFactor=fullMetricsFactor,
        ActiveThreshold=(
            activeThreshold if activeThreshold is not None else pmtc.ActiveThreshold
        ),
    )


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    pid_tid_list = get_pid_tid_list(procfs_root=procfs_root)
    pmtc_list = [
        load_pmtc(pid, tid=tid, procfs_root=procfs_root) for pid, tid in pid_tid_list
    ]

    # Select the test case w/ the highest [US]Time changes, best for delta
    # changes where %CPU is concerned:
    max_cputime = None
    pmtc_used_for_delta = None
    for all_non_zero in [True, False]:
        for pmtc in pmtc_list:
            procStat = pmtc.WantPmce.ProcStat
            cputimes = [procStat.UTime, procStat.STime]
            if all_non_zero and not all(cputimes):
                continue
            cputime = sum(cputimes)
            if max_cputime is None or cputime > max_cputime:
                pmtc_used_for_delta = pmtc
        if pmtc_used_for_delta is not None:
            break

    # Generate the simple (no prior info) test case file:
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in pmtc_list],
        os.path.join(test_case_dir, PID_METRICS_TEST_CASES_FILE_NAME),
    )

    # Generate the delta test case file for case by case run, which may be all
    # based on the same PID, TID:
    delta_pmtc_list = []

    # No change:
    # - for a non full refresh cycle:
    delta_pmtc_list.append(make_no_change_pmtc(pmtc_used_for_delta))
    # - for a full refresh cycle, above active threshold:
    delta_pmtc_list.append(
        make_no_change_pmtc(
            pmtc_used_for_delta,
            fullMetricsFactor=2,
            refreshCycleNum=0,
            activeThreshold=0,
        )
    )
    # - for a full refresh cycle, below active threshold:
    delta_pmtc_list.append(
        make_no_change_pmtc(
            pmtc_used_for_delta,
            fullMetricsFactor=2,
            refreshCycleNum=0,
            activeThreshold=GUARANTEED_INACTIVE_THRESHOLD,
        )
    )

    # Active threshold tests:
    delta_pmtc_list.extend(make_active_threshold_pmtc(pmtc_used_for_delta))

    # One delta at the time:
    stat_field_spec_list = get_stat_field_spec_list(pmtc_used_for_delta)
    for (stat_field, stat_field_spec) in stat_field_spec_list:
        pmtc = make_delta_pmtc(pmtc_used_for_delta, stat_field, stat_field_spec)
        if pmtc is not None:
            delta_pmtc_list.append(pmtc)
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in delta_pmtc_list],
        os.path.join(test_case_dir, PID_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )

    # Generate delta test cases for each PID, TID to be used for
    # GenerateAllPidMetrics; for each pid, tid rotate the stat and the stat
    # field:
    delta_pmtc_list = []
    for i, pmtc in enumerate(pmtc_list):
        stat_field, stat_field_spec = stat_field_spec_list[
            i % len(stat_field_spec_list)
        ]
        pmtc = make_delta_pmtc(pmtc, stat_field, stat_field_spec)
        if pmtc is not None:
            delta_pmtc_list.append(pmtc)
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in delta_pmtc_list],
        os.path.join(test_case_dir, ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
