#! /usr/bin/env python3

# support for pvmi/proc_net_dev_metrics_test.go

import dataclasses
import os
from copy import deepcopy
from typing import Dict, List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from . import proc_net_dev_metrics
from .common import Metric

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME = "proc_net_dev_metrics_test_cases.json"
PROC_NET_DEV_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_net_dev_metrics_delta_test_cases.json"
)


@dataclasses.dataclass
class ProcNetDevMetricsTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevNetDev: Optional[procfs.NetDev] = None
    PrevTimestamp: int = 0
    WantNetDev: Optional[procfs.NetDev] = None
    Timestamp: int = 0
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0
    RefreshGroupNum: Optional[Dict[str, int]] = None
    NextRefreshGroupNum: int = 0


def load_pndtc(
    name: str = "",
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
    refresh_cycle_num: int = 0,
    refresh_group_num: Optional[Dict[str, int]] = None,
    next_refresh_group_num: int = 0,
) -> ProcNetDevMetricsTestCase:
    proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    return ProcNetDevMetricsTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantNetDev=proc_net_dev,
        Timestamp=proc_net_dev._ts,
        WantMetrics=proc_net_dev_metrics.proc_net_dev_metrics(proc_net_dev),
        FullMetricsFactor=full_metrics_factor,
        RefreshCycleNum=refresh_cycle_num,
        RefreshGroupNum=refresh_group_num,
        NextRefreshGroupNum=next_refresh_group_num,
    )


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    pndtc_list = []
    tc = load_pndtc(name="full", procfs_root=procfs_root)
    for full_metrics_factor in range(4):
        tcf = deepcopy(tc)
        tcf.FullMetricsFactor = full_metrics_factor
        pndtc_list.append(tcf)
    save_to_json_file(
        [pndtc.to_json_compat() for pndtc in pndtc_list],
        os.path.join(test_case_dir, PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME),
    )
