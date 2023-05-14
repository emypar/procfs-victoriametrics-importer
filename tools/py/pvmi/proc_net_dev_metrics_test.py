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
from .common import Metric, metrics_delta, ts_to_go_time

# The following should match pvmi/proc_stat_metrics_test.go:
PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME = "proc_net_dev_metrics_test_cases.json"
PROC_NET_DEV_METRICS_DELTA_TEST_CASES_FILE_NAME = (
    "proc_net_dev_metrics_delta_test_cases.json"
)
PROC_NET_DEV_METRICS_DELTA_INTERVAL_SECOND = 1
PROC_NET_DEV_REMOVED_DEVICE_NAME = "__removed__"


@dataclasses.dataclass
class ProcNetDevMetricsTestCase(StructBase):
    Name: str = ""
    ProcfsRoot: str = TestdataProcfsRoot
    PrevNetDev: Optional[procfs.NetDev] = None
    PrevRxBps: Optional[Dict[str, int]] = None
    PrevTxBps: Optional[Dict[str, int]] = None
    PrevTimestamp: str = dataclasses.field(default_factory=ts_to_go_time)
    PrevRefreshGroupNum: Optional[Dict[str, int]] = None
    PrevNextRefreshGroupNum: int = 0
    WantNetDev: Optional[procfs.NetDev] = None
    WantRxBps: Optional[Dict[str, int]] = None
    WantTxBps: Optional[Dict[str, int]] = None
    Timestamp: str = dataclasses.field(default_factory=ts_to_go_time)
    WantMetrics: List[Metric] = dataclasses.field(default_factory=list)
    FullMetricsFactor: int = 0
    RefreshCycleNum: int = 0
    WantRefreshGroupNum: Optional[Dict[str, int]] = None
    WantNextRefreshGroupNum: int = 0


def load_pndtc(
    name: str = "full",
    proc_net_dev: Optional[procfs.NetDev] = None,
    procfs_root: str = TestdataProcfsRoot,
    full_metrics_factor: int = 0,
) -> ProcNetDevMetricsTestCase:
    if proc_net_dev is None:
        proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    for proc_net_dev_line in proc_net_dev.values():
        ts = proc_net_dev_line._ts
        break
    tc = ProcNetDevMetricsTestCase(
        Name=name,
        ProcfsRoot=rel_path_to_file(procfs_root),
        WantNetDev=proc_net_dev,
        Timestamp=ts_to_go_time(ts),
        WantMetrics=proc_net_dev_metrics.proc_net_dev_metrics(proc_net_dev),
        FullMetricsFactor=full_metrics_factor,
    )
    if tc.FullMetricsFactor > 1:
        tc.WantRefreshGroupNum = {}
        tc.WantNextRefreshGroupNum = tc.PrevNextRefreshGroupNum
        for device in sorted(tc.WantNetDev):
            tc.WantRefreshGroupNum[device] = tc.WantNextRefreshGroupNum
            tc.WantNextRefreshGroupNum += 1
            if tc.WantNextRefreshGroupNum >= tc.FullMetricsFactor:
                tc.WantNextRefreshGroupNum = 0
    return tc


def make_full_refresh_pndtc(
    name: str = "full-refresh",
    proc_net_dev: Optional[procfs.NetDev] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetDevMetricsTestCase]:
    pndtc_list = []
    if proc_net_dev is None:
        proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    for proc_net_dev_line in proc_net_dev.values():
        ts = proc_net_dev_line._ts
        break
    timestamp = ts_to_go_time(ts)
    prev_ts = ts - PROC_NET_DEV_METRICS_DELTA_INTERVAL_SECOND
    prev_timestamp = ts_to_go_time(prev_ts)
    device_list = sorted(proc_net_dev)
    want_next_refresh_group_num = len(proc_net_dev)
    rx_bps = {device: 0 for device in device_list}
    tx_bps = {device: 0 for device in device_list}
    for full_metrics_factor in range(1, len(proc_net_dev) + 2):
        refresh_group_num = {
            device: i % full_metrics_factor for (i, device) in enumerate(device_list)
        }
        want_next_refresh_group_num = len(refresh_group_num) % full_metrics_factor
        for refresh_cycle_num in range(full_metrics_factor):
            refresh_device_list = [
                device
                for device in device_list
                if refresh_group_num[device] == refresh_cycle_num
            ]
            want_metrics = []
            for device in refresh_device_list:
                net_dev_line = proc_net_dev[device]
                want_metrics.extend(
                    proc_net_dev_metrics.proc_net_dev_line_metrics(
                        net_dev_line,
                        prev_net_dev_line=net_dev_line,
                        prev_ts=prev_ts,
                    )
                )
            tc = ProcNetDevMetricsTestCase(
                Name=f'{name}:{"+".join(refresh_device_list)}',
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevNetDev=proc_net_dev,
                PrevRxBps=rx_bps,
                PrevTxBps=tx_bps,
                PrevTimestamp=prev_timestamp,
                PrevRefreshGroupNum=refresh_group_num,
                PrevNextRefreshGroupNum=want_next_refresh_group_num,
                WantNetDev=proc_net_dev,
                WantRxBps=rx_bps,
                WantTxBps=tx_bps,
                Timestamp=timestamp,
                WantMetrics=want_metrics,
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
                WantRefreshGroupNum=refresh_group_num,
                WantNextRefreshGroupNum=want_next_refresh_group_num,
            )
            pndtc_list.append(tc)
    return pndtc_list


def make_removed_device_pndtc(
    name: str = "removed-device",
    proc_net_dev: Optional[procfs.NetDev] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> ProcNetDevMetricsTestCase:
    if proc_net_dev is None:
        proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    for proc_net_dev_line in proc_net_dev.values():
        ts = proc_net_dev_line._ts
        break
    timestamp = ts_to_go_time(ts)
    prev_ts = ts - PROC_NET_DEV_METRICS_DELTA_INTERVAL_SECOND
    prev_timestamp = ts_to_go_time(prev_ts)
    prev_proc_net_dev = deepcopy(proc_net_dev)
    prev_proc_net_dev[PROC_NET_DEV_REMOVED_DEVICE_NAME] = procfs.NetDevLine(
        name=PROC_NET_DEV_REMOVED_DEVICE_NAME
    )
    prev_device_list = sorted(prev_proc_net_dev)
    prev_rx_bps = {device: 0 for device in prev_device_list}
    prev_tx_bps = {device: 0 for device in prev_device_list}
    # Ensure that no device will have a full refresh cycle:
    full_metrics_factor = 2 * len(prev_device_list) + 1
    refresh_cycle_num = full_metrics_factor - 1
    prev_refresh_group_num = {
        device: i % full_metrics_factor for (i, device) in enumerate(prev_device_list)
    }
    want_next_refresh_group_num = len(prev_refresh_group_num) % full_metrics_factor
    want_refresh_group_num = deepcopy(prev_refresh_group_num)
    del want_refresh_group_num[PROC_NET_DEV_REMOVED_DEVICE_NAME]
    want_rx_bps = deepcopy(prev_rx_bps)
    del want_rx_bps[PROC_NET_DEV_REMOVED_DEVICE_NAME]
    want_tx_bps = deepcopy(prev_tx_bps)
    del want_tx_bps[PROC_NET_DEV_REMOVED_DEVICE_NAME]
    tc = ProcNetDevMetricsTestCase(
        Name=f"{name}:{PROC_NET_DEV_REMOVED_DEVICE_NAME}",
        ProcfsRoot=rel_path_to_file(procfs_root),
        PrevNetDev=prev_proc_net_dev,
        PrevRxBps=prev_rx_bps,
        PrevTxBps=prev_tx_bps,
        PrevTimestamp=prev_timestamp,
        PrevRefreshGroupNum=prev_refresh_group_num,
        PrevNextRefreshGroupNum=want_next_refresh_group_num,
        WantNetDev=proc_net_dev,
        WantRxBps=want_rx_bps,
        WantTxBps=want_tx_bps,
        Timestamp=timestamp,
        WantMetrics=[],
        FullMetricsFactor=full_metrics_factor,
        RefreshCycleNum=refresh_cycle_num,
        WantRefreshGroupNum=want_refresh_group_num,
        WantNextRefreshGroupNum=want_next_refresh_group_num,
    )
    return tc


def make_delta_pndtc(
    name: str = "delta",
    proc_net_dev: Optional[procfs.NetDev] = None,
    procfs_root: str = TestdataProcfsRoot,
) -> List[ProcNetDevMetricsTestCase]:
    pndtc_list = []
    if proc_net_dev is None:
        proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    for proc_net_dev_line in proc_net_dev.values():
        ts = proc_net_dev_line._ts
        break
    timestamp = ts_to_go_time(ts)
    prev_ts = ts - PROC_NET_DEV_METRICS_DELTA_INTERVAL_SECOND
    prev_prev_ts = prev_ts - (ts - prev_ts)
    prev_timestamp = ts_to_go_time(prev_ts)
    device_list = sorted(proc_net_dev)
    refresh_group_num = {device: i for (i, device) in enumerate(device_list)}
    next_refresh_group_num = len(proc_net_dev)
    # Ensure that no device gets full refresh:
    full_metrics_factor = 2 * len(device_list) + 1
    refresh_cycle_num = full_metrics_factor - 1
    prev_rx_bps = {device: 0 for device in device_list}
    prev_tx_bps = {device: 0 for device in device_list}
    for device in device_list:
        proc_net_dev_line = proc_net_dev[device]
        for field_spec in proc_net_dev_line.get_field_spec_list():
            val = proc_net_dev_line.get_field(field_spec)
            if not isinstance(val, (int, float)):
                continue
            if field_spec in {"rx_bytes", "tx_byes"} and val == 0:
                continue
            prev_val = 0 if val != 0 else 1
            prev_proc_net_dev_line = deepcopy(proc_net_dev_line)
            prev_proc_net_dev_line.set_field(field_spec, prev_val)
            prev_proc_net_dev_line._ts = prev_ts
            prev_proc_net_dev = deepcopy(proc_net_dev)
            prev_proc_net_dev[proc_net_dev_line.name] = prev_proc_net_dev_line
            want_metrics = metrics_delta(
                proc_net_dev_metrics.proc_net_dev_line_metrics(
                    prev_proc_net_dev_line,
                    prev_net_dev_line=prev_proc_net_dev_line,
                    prev_ts=prev_prev_ts,
                ),
                proc_net_dev_metrics.proc_net_dev_line_metrics(
                    proc_net_dev_line,
                    prev_net_dev_line=prev_proc_net_dev_line,
                ),
            )
            want_rx_bps = deepcopy(prev_rx_bps)
            if field_spec == "rx_bytes":
                want_rx_bps[device] = int((val - prev_val) / (ts - prev_ts) * 8)
            want_tx_bps = deepcopy(prev_tx_bps)
            if field_spec == "tx_bytes":
                want_tx_bps[device] = int((val - prev_val) / (ts - prev_ts) * 8)
            tc = ProcNetDevMetricsTestCase(
                Name=f"{name}:{device}:{field_spec}:{prev_val}->{val}",
                ProcfsRoot=rel_path_to_file(procfs_root),
                PrevNetDev=prev_proc_net_dev,
                PrevRxBps=prev_rx_bps,
                PrevTxBps=prev_tx_bps,
                PrevTimestamp=prev_timestamp,
                PrevRefreshGroupNum=refresh_group_num,
                PrevNextRefreshGroupNum=next_refresh_group_num,
                WantNetDev=proc_net_dev,
                WantRxBps=want_rx_bps,
                WantTxBps=want_tx_bps,
                Timestamp=timestamp,
                WantMetrics=want_metrics,
                FullMetricsFactor=full_metrics_factor,
                RefreshCycleNum=refresh_cycle_num,
                WantRefreshGroupNum=refresh_group_num,
                WantNextRefreshGroupNum=next_refresh_group_num,
            )
            pndtc_list.append(tc)
    return pndtc_list


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    pndtc_list = []
    proc_net_dev = procfs.load_net_dev(procfs_root=procfs_root)
    for full_metrics_factor in [0, 1, len(proc_net_dev) + 1]:
        pndtc_list.append(
            load_pndtc(
                proc_net_dev=proc_net_dev, full_metrics_factor=full_metrics_factor
            )
        )
    pndtc_list.extend(make_full_refresh_pndtc(proc_net_dev=proc_net_dev))
    pndtc_list.append(make_removed_device_pndtc(proc_net_dev=proc_net_dev))
    save_to_json_file(
        [pndtc.to_json_compat() for pndtc in pndtc_list],
        os.path.join(test_case_dir, PROC_NET_DEV_METRICS_TEST_CASES_FILE_NAME),
    )

    pndtc_list = make_delta_pndtc(proc_net_dev=proc_net_dev)
    save_to_json_file(
        [pndtc.to_json_compat() for pndtc in pndtc_list],
        os.path.join(test_case_dir, PROC_NET_DEV_METRICS_DELTA_TEST_CASES_FILE_NAME),
    )
