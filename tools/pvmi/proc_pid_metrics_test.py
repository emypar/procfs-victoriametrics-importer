#! /usr/bin/env python3

# support for pvmi/proc_pid_metrics.go testing

import dataclasses
import os
from copy import deepcopy
from typing import List, Optional, Tuple, Union

import procfs
from metrics_common_test import TestClktckSec, TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import (
    StructBase,
    rel_path_to_file,
    save_to_json_file,
    ts_to_prometheus_ts,
)

from .common import Metric
from .pid_list import get_pid_tid_list
from .proc_pid_metrics import (
    PidMetricsCacheEntry,
    PmceStatFieldName,
    generate_pcme_pseudo_categorical_change_metrics,
    generate_pmce_full_metrics,
    load_pmce,
    pmce_field_to_metrics_fn_map,
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
GUARANTEED_INACTIVE_THRESHOLD = 2 * int(PID_SCAN_INTERVAL_SECONDS / TestClktckSec + 1)


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
    pid: int, tid: int = 0, name: str = "", procfs_root: str = TestdataProcfsRoot
) -> PidMetricsTestCase:
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


def clone_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "no-change",
    fullMetricsFactor: Optional[int] = 15,
    activeThreshold: Optional[int] = 0,
) -> PidMetricsTestCase:
    """Create a new delta testcase with no changes"""
    want_pmce = deepcopy(pmtc.WantPmce)
    if want_pmce.PassNum <= 1:
        want_pmce.PassNum = 2
    prev_pmce = deepcopy(want_pmce)
    prev_pmce.Timestamp -= ts_to_prometheus_ts(PID_SCAN_INTERVAL_SECONDS)
    for pmce_stat_field in pmce_field_to_metrics_fn_map:
        getattr(prev_pmce, pmce_stat_field)._ts = prev_pmce.Timestamp
    prev_pmce.PassNum -= 1
    if fullMetricsFactor is None:
        fullMetricsFactor = pmtc.FullMetricsFactor
    if fullMetricsFactor > 1:
        prev_pmce.FullMetricsCycleNum = fullMetricsFactor
        want_pmce.FullMetricsCycleNum = 1
    else:
        prev_pmce.FullMetricsCycleNum = 0
        want_pmce.FullMetricsCycleNum = 0
    return PidMetricsTestCase(
        Name=name if name is not None else pmtc.Name,
        Pid=pmtc.Pid,
        Tid=pmtc.Tid,
        ProcfsRoot=pmtc.ProcfsRoot,
        PrevPmce=prev_pmce,
        WantPmce=want_pmce,
        FullMetricsFactor=fullMetricsFactor,
        ActiveThreshold=(
            activeThreshold if activeThreshold is not None else pmtc.ActiveThreshold
        ),
    )


def make_full_metrics_pmtc(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "full-metrics",
    fullMetricsFactor: Optional[int] = 15,
    activeThreshold: Optional[int] = 0,
) -> PidMetricsTestCase:
    """Create a new delta test case set for full metrics cycle"""
    new_pmtc = clone_pmtc(
        pmtc,
        name=name,
        fullMetricsFactor=fullMetricsFactor,
        activeThreshold=activeThreshold,
    )
    new_pmtc.PrevPmce.FullMetricsCycleNum = new_pmtc.FullMetricsFactor - 1
    new_pmtc.WantPmce.FullMetricsCycleNum = new_pmtc.FullMetricsFactor
    update_pmtc_metrics(
        new_pmtc,
        generate_pmce_full_metrics(new_pmtc.WantPmce, prev_pmce=new_pmtc.PrevPmce),
    )
    return new_pmtc


def make_delta_pmtc(
    pmtc: PidMetricsTestCase,
    pmce_stat_field: PmceStatFieldName,
    stat_field_spec: procfs.ProcfsStructFieldSpec,
    name: Optional[str] = None,
    fullMetricsFactor: Optional[int] = 15,
    activeThreshold: Optional[int] = 0,
) -> PidMetricsTestCase:
    """Build a delta test case where just one field of a given stat changes"""

    if name is None:
        try:
            name = getattr(pmtc.WantPmce, pmce_stat_field).__class__.__name__
        except (AttributeError, ValueError):
            name = pmce_stat_field
    new_pmtc = clone_pmtc(
        pmtc,
        name=f"{name}:{stat_field_spec}",
        fullMetricsFactor=fullMetricsFactor,
        activeThreshold=activeThreshold,
    )
    if pmce_stat_field == "_procCgroups":
        update_pmce(new_pmtc.PrevPmce, rawCgroup=PREV_PROC_PID_CGROUP_DELTA)
        update_pmtc_metrics(
            new_pmtc,
            generate_pcme_pseudo_categorical_change_metrics(
                new_pmtc.WantPmce,
                new_pmtc.PrevPmce,
                clear_only=False,
            ),
        )
    elif pmce_stat_field == "_procCmdline":
        update_pmce(new_pmtc.PrevPmce, rawCmdline=PREV_PROC_PID_CMDLINE_DELTA)
        update_pmtc_metrics(
            new_pmtc,
            generate_pcme_pseudo_categorical_change_metrics(
                new_pmtc.WantPmce,
                new_pmtc.PrevPmce,
                clear_only=False,
            ),
        )
    else:
        stat, prev_stat, common_labels = (
            getattr(new_pmtc.WantPmce, pmce_stat_field),
            getattr(new_pmtc.PrevPmce, pmce_stat_field),
            new_pmtc.WantPmce.CommonLabels,
        )
        if pmce_stat_field == "ProcStat" and stat_field_spec in {"UTime", "STime"}:
            # Special case for XTime since is it can only be decreased. If
            # already 0 then no delta will be generated for this field.
            xtime = prev_stat.get_field(stat_field_spec)
            if xtime == 0:
                return new_pmtc
            # Aim for 50% CPU:
            d_xtime = int(PID_SCAN_INTERVAL_SECONDS / TestClktckSec / 2)
            prev_stat.set_field(stat_field_spec, max(0, xtime - d_xtime))
        else:
            alter_procfs_struct_field(prev_stat, stat_field_spec)
        update_pmce(new_pmtc.PrevPmce)
        metrics_fn_map = pmce_field_to_metrics_fn_map[pmce_stat_field]
        ts = new_pmtc.WantPmce.Timestamp
        for metric_fn, require_history in metrics_fn_map[stat_field_spec]:
            if require_history:
                metric_or_metrics = metric_fn(stat, prev_stat, common_labels, ts=ts)
            else:
                metric_or_metrics = metric_fn(stat, common_labels, ts=ts)
            update_pmtc_metrics(new_pmtc, metric_or_metrics)
        update_pmtc_metrics(
            new_pmtc,
            generate_pcme_pseudo_categorical_change_metrics(
                new_pmtc.WantPmce,
                new_pmtc.PrevPmce,
                clear_only=True,
            ),
        )
    return new_pmtc


def make_active_threshold_test(
    pmtc: PidMetricsTestCase,
    name: Optional[str] = "active-threshold",
) -> List[PidMetricsTestCase]:

    # Generate changes to ProcStat, ProcStatus and ProcIo and use them for 2
    # test cases: above and below active threshold. For the former only
    # ProcStat's ones should show in the generated metrics whereas for the
    # latter all should.
    inactive_stat_field_spec_list = [("ProcStat", "RSS")]
    active_stat_field_spec_list = inactive_stat_field_spec_list + [
        ("ProcStatus", "VmSize"),
        ("ProcIo", "RChar"),
    ]

    pmtc_list = []
    for active_threshold, stat_field_spec_list in [
        (0, active_stat_field_spec_list),
        (GUARANTEED_INACTIVE_THRESHOLD, inactive_stat_field_spec_list),
    ]:
        new_pmtc = clone_pmtc(
            pmtc,
            name=name,
            fullMetricsFactor=15,
            activeThreshold=active_threshold,
        )
        for pmce_stat_field, stat_field_spec in stat_field_spec_list:
            stat, prev_stat, common_labels = (
                getattr(new_pmtc.WantPmce, pmce_stat_field),
                getattr(new_pmtc.PrevPmce, pmce_stat_field),
                new_pmtc.WantPmce.CommonLabels,
            )
            alter_procfs_struct_field(prev_stat, stat_field_spec)
            for metric_fn, _ in pmce_field_to_metrics_fn_map[pmce_stat_field][
                stat_field_spec
            ]:
                update_pmtc_metrics(new_pmtc, metric_fn(stat, common_labels))
        pmtc_list.append(new_pmtc)
    return pmtc_list


def get_stat_field_spec_pairs() -> List[
    Tuple[PmceStatFieldName, procfs.ProcfsStructFieldSpec]
]:
    return [
        (pmce_stat_field, field_spec)
        for pmce_stat_field in sorted(pmce_field_to_metrics_fn_map)
        for field_spec in sorted(
            pmce_field_to_metrics_fn_map[pmce_stat_field],
            key=lambda fs: (fs, -1)
            if not isinstance(fs, tuple) or fs[1] is None
            else fs,
        )
    ]


def stat_field_spec_pair_generator(
    stat_field_spec_pair_list: Tuple[PmceStatFieldName, procfs.ProcfsStructFieldSpec],
    n: int,
):
    """Each next should rotate the field stat"""

    by_field = {}
    for pmce_stat_field, stat_field_spec in stat_field_spec_pair_list:
        if pmce_stat_field not in by_field:
            by_field[pmce_stat_field] = []
        by_field[pmce_stat_field].append(stat_field_spec)
    fields = sorted(by_field)
    by_field_index = {pmce_stat_field: 0 for pmce_stat_field in by_field}
    for i in range(n):
        pmce_stat_field = fields[i % len(fields)]
        stat_field_spec_index = by_field_index[pmce_stat_field]
        by_field_index[pmce_stat_field] = (stat_field_spec_index + 1) % len(
            by_field[pmce_stat_field]
        )
        yield pmce_stat_field, by_field[pmce_stat_field][stat_field_spec_index]


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
    delta_pmtc = None
    for all_non_zero in [True, False]:
        for pmtc in pmtc_list:
            procStat = pmtc.WantPmce.ProcStat
            cputimes = [procStat.UTime, procStat.STime]
            if all_non_zero and not all(cputimes):
                continue
            cputime = sum(cputimes)
            if max_cputime is None or cputime > max_cputime:
                delta_pmtc = pmtc
        if delta_pmtc is not None:
            break

    # Generate load test file:
    save_to_json_file(
        delta_pmtc.to_json_compat(),
        os.path.join(TestdataTestCasesDir, PID_METRICS_TEST_CASE_LOAD_FILE_NAME),
    )

    # Generate the simple (no prior info) test case file:
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in pmtc_list],
        os.path.join(TestdataTestCasesDir, PID_METRICS_TEST_CASES_FILE_NAME),
    )

    # Generate the delta test case file for case by case run, which may be all
    # based on the same PID, TID:
    delta_pmtc_list = []

    # No change:
    delta_pmtc_list.append(clone_pmtc(delta_pmtc))

    # Full metrics, regardless of active threshold:
    # - all active:
    delta_pmtc_list.append(make_full_metrics_pmtc(delta_pmtc, activeThreshold=0))
    # - none active
    delta_pmtc_list.append(
        make_full_metrics_pmtc(
            delta_pmtc,
            activeThreshold=GUARANTEED_INACTIVE_THRESHOLD,
        )
    )

    # Active threshold tests:
    delta_pmtc_list.extend(make_active_threshold_test(delta_pmtc))

    # At the present, PID cgroup and cmdline do not have deltas:
    stat_field_spec_list = [
        (pmce_stat_field, stat_field_spec)
        for (pmce_stat_field, stat_field_spec) in get_stat_field_spec_pairs()
        if pmce_stat_field not in {"_procCgroups", "_procCmdline"}
    ]

    # One delta at the time:
    for pmce_stat_field, stat_field_spec in stat_field_spec_list:
        new_pmtc = make_delta_pmtc(delta_pmtc, pmce_stat_field, stat_field_spec)
        if new_pmtc is not None:
            delta_pmtc_list.append(new_pmtc)
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in delta_pmtc_list],
        os.path.join(TestdataTestCasesDir, PID_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )

    # Generate delta test cases for each PID, TID to be used for
    # GenerateAllPidMetrics; for each pid, tid rotate the stat:
    delta_pmtc_list = []

    for i, (pmce_stat_field, stat_field_spec) in enumerate(
        stat_field_spec_pair_generator(stat_field_spec_list, len(pmtc_list))
    ):
        new_pmtc = make_delta_pmtc(pmtc_list[i], pmce_stat_field, stat_field_spec)
        if new_pmtc is not None:
            delta_pmtc_list.append(new_pmtc)
    save_to_json_file(
        [pmtc.to_json_compat() for pmtc in delta_pmtc_list],
        os.path.join(TestdataTestCasesDir, ALL_PID_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
