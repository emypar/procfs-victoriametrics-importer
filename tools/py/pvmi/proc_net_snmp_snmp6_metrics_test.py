#! /usr/bin/env python3

# support for pvmi/proc_net_snmp_snmp6_metrics_test.go

import dataclasses
import os
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from .common import Metric, ts_to_go_time
from .proc_net_snmp_snmp6_metrics import proc_net_snmp6_metrics, proc_net_snmp_metrics

# The following should match pvmi/proc_net_snmp_snmp6_metrics_test.go:
PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME = "proc_net_snmp_metrics_test_cases.json"
PROC_NET_SNMP_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_net_snmp_metrics_delta_test_cases.json"
)
PROC_NET_SNMP6_METRICS_TEST_CASES_FILE_NAME = "proc_net_snmp6_metrics_test_cases.json"
PROC_NET_SNMP6_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_net_snmp6_metrics_delta_test_cases.json"
)


@dataclasses.dataclass
class ProcNetSnmpTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevNetSnmp: Optional[procfs.FixedLayoutDataModel] = None
    PrevNetSnmp6: Optional[procfs.FixedLayoutDataModel] = None
    WantNetSnmp: Optional[procfs.FixedLayoutDataModel] = None
    WantNetSnmp6: Optional[procfs.FixedLayoutDataModel] = None
    Timestamp: str = ""
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0


def load_fldm(
    procfs_root: str = TestdataProcfsRoot,
    is_snmp6: bool = False,
) -> procfs.FixedLayoutDataModel:
    return (
        procfs.load_net_snmp6_data_model(procfs_root=procfs_root)
        if is_snmp6
        else procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    )


def get_metrics(
    fldm: procfs.FixedLayoutDataModel,
    is_snmp6: bool = False,
) -> List[Metric]:
    return proc_net_snmp6_metrics(fldm) if is_snmp6 else proc_net_snmp_metrics(fldm)


def set_fldm(
    tc: ProcNetSnmpTestCase,
    fldm: procfs.FixedLayoutDataModel,
    prev_fldm: Optional[procfs.FixedLayoutDataModel] = None,
    is_snmp6: bool = False,
) -> ProcNetSnmpTestCase:
    if is_snmp6:
        tc.PrevNetSnmp6 = prev_fldm
        tc.WantNetSnmp6 = fldm
    else:
        tc.PrevNetSnmp = prev_fldm
        tc.WantNetSnmp = fldm
    return tc


def make_full_tc(
    name: str = "full",
    fldm: Optional[procfs.FixedLayoutDataModel] = None,
    is_snmp6: bool = False,
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
) -> ProcNetSnmpTestCase:
    if fldm is None:
        fldm = load_fldm(procfs_root=procfs_root, is_snmp6=is_snmp6)
    ts = fldm._ts
    timestamp = ts_to_go_time(ts)
    tc = set_fldm(
        ProcNetSnmpTestCase(
            Name=name,
            ProcfsRoot=rel_path_to_file(procfs_root),
            Timestamp=timestamp,
            WantMetrics=get_metrics(fldm, is_snmp6=is_snmp6),
            FullMetricsFactor=full_metrics_factor,
        ),
        fldm,
        is_snmp6=is_snmp6,
    )
    return tc


def make_refresh_tc(
    name: str = "refresh",
    is_snmp6: bool = False,
    fldm: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetSnmpTestCase]:
    tc_list = []
    if fldm is None:
        fldm = load_fldm(procfs_root=procfs_root, is_snmp6=is_snmp6)
    ts = fldm._ts
    timestamp = ts_to_go_time(ts)
    metrics = get_metrics(fldm, is_snmp6=is_snmp6)
    for full_metrics_factor in range(
        0, len(fldm.Values) + 1, len(fldm.Values) // 13 + 1
    ):
        for refresh_cycle_num in range(max(full_metrics_factor, 1)):
            want_metrics = [
                metrics[i]
                for i in range(len(metrics))
                if full_metrics_factor <= 1
                or (i % full_metrics_factor == refresh_cycle_num)
            ]
            tc_list.append(
                set_fldm(
                    ProcNetSnmpTestCase(
                        Name=name,
                        ProcfsRoot=rel_path_to_file(procfs_root),
                        Timestamp=timestamp,
                        WantMetrics=want_metrics,
                        FullMetricsFactor=full_metrics_factor,
                        RefreshCycleNum=refresh_cycle_num,
                    ),
                    fldm,
                    prev_fldm=fldm,
                    is_snmp6=is_snmp6,
                )
            )
    return tc_list


def make_delta_tc(
    name: str = "delta",
    is_snmp6: bool = False,
    fldm: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetSnmpTestCase]:
    tc_list = []
    if fldm is None:
        fldm = load_fldm(procfs_root=procfs_root, is_snmp6=is_snmp6)
    ts = fldm._ts
    timestamp = ts_to_go_time(ts)
    metrics = get_metrics(fldm, is_snmp6=is_snmp6)
    full_metrics_factor = len(fldm.Values)
    for refresh_cycle_num in range(max(full_metrics_factor, 1)):
        prev_fldm = procfs.FixedLayoutDataModel(
            Values=list(fldm.Values), Names=fldm.Names, Groups=fldm.Groups, _ts=ts - 1
        )
        for i in range(len(metrics)):
            if full_metrics_factor > 1 and (
                i % full_metrics_factor != refresh_cycle_num
            ):
                continue
            prev_fldm.Values[i] = "0" if fldm.Values != "0" else "-1"
            want_metrics = metrics[i : i + 1]
            tc_list.append(
                set_fldm(
                    ProcNetSnmpTestCase(
                        Name=name,
                        ProcfsRoot=rel_path_to_file(procfs_root),
                        Timestamp=timestamp,
                        WantMetrics=want_metrics,
                        FullMetricsFactor=full_metrics_factor,
                        RefreshCycleNum=refresh_cycle_num,
                    ),
                    fldm,
                    prev_fldm=prev_fldm,
                    is_snmp6=is_snmp6,
                )
            )
    return tc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    for is_snmp6 in [False, True]:
        fldm = load_fldm(procfs_root=procfs_root, is_snmp6=is_snmp6)
        tc_list = []
        for full_metrics_factor in [0, 1, len(fldm.Values) + 1]:
            tc_list.append(
                make_full_tc(
                    fldm=fldm,
                    is_snmp6=is_snmp6,
                    full_metrics_factor=full_metrics_factor,
                )
            )
        tc_list.extend(make_refresh_tc(fldm=fldm, is_snmp6=is_snmp6))
        save_to_json_file(
            [tc.to_json_compat() for tc in tc_list],
            os.path.join(
                test_case_dir,
                PROC_NET_SNMP6_METRICS_TEST_CASES_FILE_NAME
                if is_snmp6
                else PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME,
            ),
        )

        tc_list = make_delta_tc(fldm=fldm, is_snmp6=is_snmp6)
        save_to_json_file(
            [tc.to_json_compat() for tc in tc_list],
            os.path.join(
                test_case_dir,
                PROC_NET_SNMP6_METRICS_DELTA_TEST_CASES_FILE_NAME
                if is_snmp6
                else PROC_NET_SNMP_METRICS_DELTA_TEST_CASES_FILE_NAME,
            ),
        )
