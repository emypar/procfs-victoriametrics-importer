#! /usr/bin/env python3


import argparse
import os
import pprint
import sys

sys.path.append(
    os.path.dirname(os.path.dirname(os.path.normpath(os.path.abspath(__file__))))
)

import procfs
import pvmi
from metrics_common_test import TestdataProcfsRoot


def print_proc_pid_metrics(
    pid: int, tid: int = 0, procfs_root: str = TestdataProcfsRoot
):
    common_labels = procfs.make_common_labels(pid, tid=tid)
    procCgroups = procfs.load_proc_pid_cgroups(pid, tid=tid)
    procCmdline = procfs.load_proc_pid_cmdline(pid, tid=tid)
    procIO = procfs.load_proc_pid_io(pid, tid=tid)
    procStat = procfs.load_proc_pid_stat(pid, tid=tid)
    procStatus = procfs.load_proc_pid_status(pid, tid=tid)

    for s, metrics_fn_map in [
        (procCgroups, pvmi.proc_pid_cgroup_metrics.metrics_fn_map),
        (procCmdline, pvmi.proc_pid_cmdline_metrics.metrics_fn_map),
        (procIO, pvmi.proc_pid_io_metrics.metrics_fn_map),
        (procStat, pvmi.proc_pid_stat_metrics.metrics_fn_map),
        (procStatus, pvmi.proc_pid_status_metrics.metrics_fn_map),
    ]:
        print("-" * 60)
        pprint.pprint(s)
        pprint.pprint(s.to_json_compat())
        print()
        for field_spec, fn_require_history_list in metrics_fn_map.items():
            for fn, require_history in fn_require_history_list:
                if require_history:
                    continue
                if isinstance(field_spec, tuple):
                    field, indx = field_spec
                    print(f"{s.__class__.__name__}.{field}[{indx}]={getattr(s, field)[indx]}")
                elif isinstance(field_spec, str):
                    field = field_spec
                    print(f"{s.__class__.__name__}.{field}={getattr(s, field)}")
                metric_or_metrics = fn(s, common_labels)
                pprint.pprint(metric_or_metrics)
                if isinstance(metric_or_metrics, list):
                    pprint.pprint([m.asstr() for m in metric_or_metrics])
                else:
                    print(metric_or_metrics.asstr())
                print()


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    parser.add_argument("pid_tid", type=str, nargs="+")
    args = parser.parse_args()

    for pid_tid in args.pid_tid:
        pid_tid = list(map(int, pid_tid.split(",")))
        pid = pid_tid[0]
        tid = pid_tid[1] if len(pid_tid) > 1 else 0
    print_proc_pid_metrics(pid, tid=tid, procfs_root=args.procfs_root)
