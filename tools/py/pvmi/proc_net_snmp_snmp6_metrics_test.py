#! /usr/bin/env python3

# support for pvmi/proc_net_snmp_snmp6_metrics_test.go

import dataclasses
import os
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from .common import Metric, ts_to_go_time
from .proc_net_snmp_snmp6_metrics import (
    net_snmp6_metric_name_map,
    net_snmp_metric_name_map,
    proc_net_snmp6_metrics,
    proc_net_snmp_metrics,
)

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


def load_net_snmp_tc(
    name: str = "full",
    net_snmp: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
) -> ProcNetSnmpTestCase:
    if net_snmp is None:
        net_snmp = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    ts = net_snmp._ts
    timestamp = ts_to_go_time(ts)
    tc = ProcNetSnmpTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantNetSnmp=net_snmp,
        Timestamp=timestamp,
        WantMetrics=proc_net_snmp_metrics(net_snmp),
        FullMetricsFactor=full_metrics_factor,
    )
    return tc


def make_refresh_net_snmp_tc(
    name: str = "refresh",
    net_snmp: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetSnmpTestCase]:
    tc_list = []
    if net_snmp is None:
        net_snmp = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    ts = net_snmp._ts
    timestamp = ts_to_go_time(ts)
    metrics = proc_net_snmp_metrics(net_snmp)
    for full_metrics_factor in range(
        0, len(net_snmp.Values) + 1, len(net_snmp.Values) // 13 + 1
    ):
        for refresh_cycle_num in range(max(full_metrics_factor, 1)):
            keep_metric_names = set(
                net_snmp_metric_name_map[net_snmp.Names[i]]
                for i in range(len(net_snmp.Names))
                if full_metrics_factor <= 1
                or (i % full_metrics_factor == refresh_cycle_num)
            )
            want_metrics = [
                metric for metric in metrics if metric.name() in keep_metric_names
            ]
            tc_list.append(
                ProcNetSnmpTestCase(
                    Name=name,
                    ProcfsRoot=rel_path_to_file(procfs_root),
                    PrevNetSnmp=net_snmp,
                    WantNetSnmp=net_snmp,
                    Timestamp=timestamp,
                    WantMetrics=want_metrics,
                    FullMetricsFactor=full_metrics_factor,
                    RefreshCycleNum=refresh_cycle_num,
                )
            )
    return tc_list


def load_net_snmp6_tc(
    name: str = "full",
    net_snmp6: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
) -> ProcNetSnmpTestCase:
    if net_snmp6 is None:
        net_snmp6 = procfs.load_net_snmp6_data_model(procfs_root=procfs_root)
    ts = net_snmp6._ts
    timestamp = ts_to_go_time(ts)
    tc = ProcNetSnmpTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantNetSnmp6=net_snmp6,
        Timestamp=timestamp,
        WantMetrics=proc_net_snmp6_metrics(net_snmp6),
        FullMetricsFactor=full_metrics_factor,
    )
    return tc


def make_refresh_net_snmp6_tc(
    name: str = "refresh",
    net_snmp6: Optional[procfs.FixedLayoutDataModel] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetSnmpTestCase]:
    tc_list = []
    if net_snmp6 is None:
        net_snmp6 = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    ts = net_snmp6._ts
    timestamp = ts_to_go_time(ts)
    metrics = proc_net_snmp6_metrics(net_snmp6)
    for full_metrics_factor in range(
        0, len(net_snmp6.Values) + 1, len(net_snmp6.Values) // 13 + 1
    ):
        for refresh_cycle_num in range(max(full_metrics_factor, 1)):
            keep_metric_names = set(
                net_snmp6_metric_name_map[net_snmp6.Names[i]]
                for i in range(len(net_snmp6.Names))
                if full_metrics_factor <= 1
                or (i % full_metrics_factor == refresh_cycle_num)
            )
            want_metrics = [
                metric for metric in metrics if metric.name() in keep_metric_names
            ]
            tc_list.append(
                ProcNetSnmpTestCase(
                    Name=name,
                    ProcfsRoot=rel_path_to_file(procfs_root),
                    PrevNetSnmp6=net_snmp6,
                    WantNetSnmp6=net_snmp6,
                    Timestamp=timestamp,
                    WantMetrics=want_metrics,
                    FullMetricsFactor=full_metrics_factor,
                    RefreshCycleNum=refresh_cycle_num,
                )
            )
    return tc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    tc_list = []
    net_snmp = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    for full_metrics_factor in [0, 1, len(net_snmp.Values) + 1]:
        tc_list.append(
            load_net_snmp_tc(net_snmp=net_snmp, full_metrics_factor=full_metrics_factor)
        )
    tc_list.extend(make_refresh_net_snmp_tc(net_snmp=net_snmp))
    save_to_json_file(
        [tc.to_json_compat() for tc in tc_list],
        os.path.join(test_case_dir, PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME),
    )

    tc_list = []
    net_snmp6 = procfs.load_net_snmp6_data_model(procfs_root=procfs_root)
    for full_metrics_factor in [0, 1, len(net_snmp6.Values) + 1]:
        tc_list.append(
            load_net_snmp6_tc(
                net_snmp6=net_snmp6, full_metrics_factor=full_metrics_factor
            )
        )
    tc_list.extend(make_refresh_net_snmp6_tc(net_snmp6=net_snmp6))
    save_to_json_file(
        [tc.to_json_compat() for tc in tc_list],
        os.path.join(test_case_dir, PROC_NET_SNMP6_METRICS_TEST_CASES_FILE_NAME),
    )
