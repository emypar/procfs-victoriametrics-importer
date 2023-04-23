#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics_test.go

import dataclasses
import os
from typing import List, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import StructBase, rel_path_to_file, save_to_json_file

from . import proc_stat_metrics
from .common import Metric

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_STAT_METRICS_TEST_CASES_FILE_NAME = "proc_stat_metrics_test_cases.json"


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


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    psmtc_list = []
    psmtc = load_psmtc(name="full")
    psmtc_list.append(psmtc)

    save_to_json_file(
        [psmtc.to_json_compat() for psmtc in psmtc_list],
        os.path.join(test_case_dir, PROC_STAT_METRICS_TEST_CASES_FILE_NAME),
    )
