#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import List, Optional, Tuple

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from .common import Metric, metrics_delta, ts_to_prometheus_ts
from .pid_list import get_pid_tid_list
from .proc_pid_metrics import (
    PidMetricsCacheEntry,
    PmceStatFieldNames,
    PmceStatFieldNameToIndexMap,
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


def make_no_change_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "no-change",
    fullMetricsFactor: Optional[int] = 15,
    refreshCycleNum: Optional[int] = None,
    activeThreshold: Optional[int] = 0,
    has_prev_cpu_pct: bool = True,
) -> PidMetricsTestCase:
    """Create a new testcase with no changes from the previous scan"""
    if name is None:
        name = pmtc.Name
    name += f",prevCpuPct={has_prev_cpu_pct}"
    want_pmce = deepcopy(pmtc.WantPmce)
    if want_pmce.PassNum <= 1:
        want_pmce.PassNum = 2
    want_pmce.ProcStat[1 - want_pmce.CrtProcStatIndex] = deepcopy(
        want_pmce.ProcStat[want_pmce.CrtProcStatIndex]
    )
    want_pmce.ProcStatus[1 - want_pmce.CrtProcStatusIndex] = deepcopy(
        want_pmce.ProcStatus[want_pmce.CrtProcStatusIndex]
    )
    want_pmce.ProcIo[1 - want_pmce.CrtProcIoIndex] = deepcopy(
        want_pmce.ProcIo[want_pmce.CrtProcIoIndex]
    )
    prev_prom_ts = want_pmce.Timestamp - ts_to_prometheus_ts(PID_SCAN_INTERVAL_SECONDS)
    update_pmce(
        want_pmce,
        prev_prom_ts=prev_prom_ts,
    )
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
    prev_pmce.CrtProcStatIndex = 1 - prev_pmce.CrtProcStatIndex
    if activeThreshold is None:
        activeThreshold = pmtc.ActiveThreshold
    if activeThreshold == 0 or fullMetricsFactor <= 1 or refreshCycleNum == 0:
        prev_pmce.CrtProcStatusIndex = 1 - prev_pmce.CrtProcStatusIndex
        prev_pmce.CrtProcIoIndex = 1 - prev_pmce.CrtProcIoIndex
    if refreshCycleNum == 0:
        want_metrics = generate_pmce_full_metrics(
            want_pmce, prev_prom_ts=prev_pmce.Timestamp
        )
    elif not has_prev_cpu_pct:
        prev_pmce.PrevSTimePct = -1.0
        prev_pmce.PrevUTimePct = -1.0
        want_metrics = metrics_delta(
            generate_pmce_full_metrics(prev_pmce),
            generate_pmce_full_metrics(want_pmce, prev_prom_ts=prev_pmce.Timestamp),
        )
    else:
        want_metrics = []
    return PidMetricsTestCase(
        Name=name,
        Pid=pmtc.Pid,
        Tid=pmtc.Tid,
        ProcfsRoot=pmtc.ProcfsRoot,
        PrevPmce=prev_pmce,
        WantPmce=want_pmce,
        WantMetrics=want_metrics,
        FullMetricsFactor=fullMetricsFactor,
        ActiveThreshold=activeThreshold,
    )


def sync_with_prev_stat(
    pmtc: PidMetricsTestCase,
    stat_field: str,
):
    """Ensure WANT.STAT[1 - WANT.CRT_INDEX] == PREV.STAT[PREV.CRT_INDEX]"""
    stat_index_field = PmceStatFieldNameToIndexMap.get(stat_field)
    if stat_index_field is None:
        return
    want_crt_index = getattr(pmtc.WantPmce, stat_index_field)
    prev_crt_index = getattr(pmtc.PrevPmce, stat_index_field)
    getattr(pmtc.WantPmce, stat_field)[1 - want_crt_index] = deepcopy(
        getattr(pmtc.PrevPmce, stat_field)[prev_crt_index]
    )


def make_active_threshold_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "active-threshold",
    has_prev_cpu_pct: bool = True,
) -> List[PidMetricsTestCase]:

    # Generate changes to ProcStat, ProcStatus and ProcIo and use them for 2
    # test cases: above and below active threshold. For the former only
    # ProcStat's ones should show in the generated metrics whereas for the
    # latter all should.

    # The following (stat, index, stat_field) will generate metrics  regardless of active state:
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
            has_prev_cpu_pct=has_prev_cpu_pct,
        )
        for stat_field, stat_field_spec in stat_field_spec_list:
            stat_index_field = PmceStatFieldNameToIndexMap.get(stat_field)
            if stat_index_field is None:
                continue
            prev_stat = getattr(new_pmtc.PrevPmce, stat_field)[
                getattr(new_pmtc.PrevPmce, stat_index_field)
            ]
            alter_procfs_struct_field(prev_stat, stat_field_spec)
            sync_with_prev_stat(new_pmtc, stat_field)
        prev_prom_ts = new_pmtc.PrevPmce.Timestamp
        prev_prev_prom_ts = new_pmtc.PrevPmce.Timestamp - (
            new_pmtc.WantPmce.Timestamp - new_pmtc.PrevPmce.Timestamp
        )
        update_pmce(new_pmtc.PrevPmce, prev_prom_ts=prev_prev_prom_ts)
        update_pmce(new_pmtc.WantPmce, prev_prom_ts=prev_prom_ts)
        new_pmtc.WantMetrics = metrics_delta(
            generate_pmce_full_metrics(
                new_pmtc.PrevPmce,
                prev_prom_ts=prev_prev_prom_ts,
            ),
            generate_pmce_full_metrics(
                new_pmtc.WantPmce,
                prev_prom_ts=prev_prom_ts,
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
        stat_index_field = PmceStatFieldNameToIndexMap.get(stat_field)
        if stat_index_field is not None:
            stat = getattr(pmtc.WantPmce, stat_field)[
                getattr(pmtc.WantPmce, stat_index_field)
            ]
            field_spec_list = (
                stat.get_field_spec_list()
                if hasattr(stat, "get_field_spec_list")
                else [None]
            )
        else:
            field_spec_list = [None]
        for field_spec in field_spec_list:
            stat_field_spec = (stat_field, field_spec)
            if stat_field_spec not in STAT_FIELD_SPEC_IGNORE_FOR_DELTA:
                stat_field_spec_list.append(stat_field_spec)
    return stat_field_spec_list


def make_delta_pmtc(
    pmtc: PidMetricsTestCase,
    stat_field: str,
    stat_field_spec: procfs.ProcfsStructFieldSpec,
    name: Optional[str] = "delta",
) -> PidMetricsTestCase:
    """Build a delta test case where just one field of a given stat changes"""

    if name is None:
        name = stat_field
    else:
        name += f":{stat_field}"
    if stat_field_spec is not None:
        name += f":{stat_field_spec}"

    # Ensure delta strategy:
    fullMetricsFactor = 2
    # Ensure active threshold:
    activeThreshold = 0
    # Ensure full metrics cycle as needed:
    if stat_field in {"RawCgroup", "RawCmdline"}:
        refreshCycleNum = 0
    else:
        refreshCycleNum = fullMetricsFactor - 1
    new_pmtc = make_no_change_pmtc(
        pmtc,
        name=name,
        fullMetricsFactor=fullMetricsFactor,
        refreshCycleNum=refreshCycleNum,
        activeThreshold=activeThreshold,
        has_prev_cpu_pct=True,
    )
    prev_prom_ts = new_pmtc.PrevPmce.Timestamp
    prev_prev_prom_ts = prev_prom_ts - (
        new_pmtc.WantPmce.Timestamp - new_pmtc.PrevPmce.Timestamp
    )
    if stat_field == "RawCgroup":
        update_pmce(new_pmtc.PrevPmce, rawCgroup=PREV_PROC_PID_CGROUP_DELTA)
    elif stat_field == "RawCmdline":
        update_pmce(new_pmtc.PrevPmce, rawCmdline=PREV_PROC_PID_CMDLINE_DELTA)
    else:
        stat_index_field = PmceStatFieldNameToIndexMap.get(stat_field)
        if stat_index_field is None:
            return None
        prev_stat = getattr(new_pmtc.PrevPmce, stat_field)[
            getattr(new_pmtc.PrevPmce, stat_index_field)
        ]
        if stat_field != "ProcStat" or stat_field_spec not in {"UTime", "STime"}:
            alter_procfs_struct_field(prev_stat, stat_field_spec)
            update_pmce(new_pmtc.PrevPmce)
            sync_with_prev_stat(new_pmtc, stat_field)
        else:
            # Delta for [US]Time require a value > 0.
            field_val = prev_stat.get_field(stat_field_spec)
            if field_val == 0:
                return None
            # Aim for 50% %CPU:
            prev_field_val = max(
                0, int(field_val - PID_SCAN_INTERVAL_SECONDS / (2 * TestClktckSec))
            )
            prev_stat.set_field(stat_field_spec, prev_field_val)
            sync_with_prev_stat(new_pmtc, stat_field)
            # This the only case where the new cache entry has to be updated
            # since %CPU is also cached:
            update_pmce(new_pmtc.WantPmce, prev_prom_ts=prev_prom_ts)
            # Disable %CPU metrics:
            prev_prev_prom_ts = None
    if refreshCycleNum == 0:
        new_pmtc.WantMetrics = generate_pmce_full_metrics(
            new_pmtc.WantPmce, prev_pmce=new_pmtc.PrevPmce
        )
    else:
        new_pmtc.WantMetrics = metrics_delta(
            generate_pmce_full_metrics(
                new_pmtc.PrevPmce, prev_prom_ts=prev_prev_prom_ts
            ),
            generate_pmce_full_metrics(new_pmtc.WantPmce, prev_pmce=new_pmtc.PrevPmce),
        )
    return new_pmtc


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    pid_tid_list = get_pid_tid_list(procfs_root=procfs_root)
    pmtc_list = [
        load_pmtc(pid, tid=tid, procfs_root=procfs_root) for pid, tid in pid_tid_list
    ]

    # Generate the simple (no prior info) test case file:
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in pmtc_list],
        os.path.join(test_case_dir, PID_METRICS_TEST_CASES_FILE_NAME),
    )

    # Select the PID only test case w/ the highest [US]Time changes, best for delta
    # changes where %CPU is concerned.
    max_cputime = None
    pmtc_used_for_delta = None
    for all_non_zero in [True, False]:
        for pmtc in pmtc_list:
            if pmtc.Tid != 0:
                continue
            procStat = pmtc.WantPmce.ProcStat[pmtc.WantPmce.CrtProcStatIndex]
            cputimes = [procStat.UTime, procStat.STime]
            if all_non_zero and not all(cputimes):
                continue
            cputime = sum(cputimes)
            if max_cputime is None or cputime > max_cputime:
                pmtc_used_for_delta = pmtc
        if pmtc_used_for_delta is not None:
            break

    # Generate the delta test case file for case by case run, which may be all
    # based on the same PID, TID:
    delta_pmtc_list = []

    # No change:
    for has_prev_cpu_pct in (False, True):
        # - for a non full refresh cycle:
        delta_pmtc_list.append(
            make_no_change_pmtc(pmtc_used_for_delta, has_prev_cpu_pct=has_prev_cpu_pct)
        )
        # - for a full refresh cycle, above active threshold:
        delta_pmtc_list.append(
            make_no_change_pmtc(
                pmtc_used_for_delta,
                fullMetricsFactor=2,
                refreshCycleNum=0,
                activeThreshold=0,
                has_prev_cpu_pct=has_prev_cpu_pct,
            )
        )
        # - for a full refresh cycle, below active threshold:
        delta_pmtc_list.append(
            make_no_change_pmtc(
                pmtc_used_for_delta,
                fullMetricsFactor=2,
                refreshCycleNum=0,
                activeThreshold=GUARANTEED_INACTIVE_THRESHOLD,
                has_prev_cpu_pct=has_prev_cpu_pct,
            )
        )

        # Active threshold tests:
        delta_pmtc_list.extend(
            make_active_threshold_pmtc(
                pmtc_used_for_delta, has_prev_cpu_pct=has_prev_cpu_pct
            )
        )

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
