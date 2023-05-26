#! /usr/bin/env python3

# support for pvmi/proc_net_snmp_snmp6_metrics_test.go

import dataclasses
import os
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from .common import Metric, ts_to_go_time
from .proc_net_snmp_snmp6_metrics import proc_net_snmp_metrics

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


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    tc_list = []
    net_snmp = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    for full_metrics_factor in [0, 1, len(net_snmp.Values) + 1]:
        tc_list.append(
            load_net_snmp_tc(
                net_snmp=net_snmp,
                full_metrics_factor=full_metrics_factor,
            )
        )
    save_to_json_file(
        [tc.to_json_compat() for tc in tc_list],
        os.path.join(test_case_dir, PROC_NET_SNMP_METRICS_TEST_CASES_FILE_NAME),
    )
