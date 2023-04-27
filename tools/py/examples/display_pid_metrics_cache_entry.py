#! /usr/bin/env python3


import argparse
import os
import pprint
import sys

sys.path.append(
    os.path.dirname(os.path.dirname(os.path.normpath(os.path.abspath(__file__))))
)

import pvmi
from metrics_common_test import TestdataProcfsRoot


def print_pid_metrics_cache_enytry(
    pid: int, tid: int = 0, procfs_root: str = TestdataProcfsRoot
):
    pmce = pvmi.proc_pid_metrics.load_pmce(pid, tid=tid, procfs_root=procfs_root)
    print(f"""{pmce.__class__.__name__}:\n""")
    pprint.pprint(pmce)
    print()
    print(f"""{pmce.__class__.__name__}.to_json_compat():\n""")
    pprint.pprint(pmce.to_json_compat())


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    parser.add_argument("pid_tid", type=str, nargs="+")
    args = parser.parse_args()

    for pid_tid in args.pid_tid:
        pid_tid = list(map(int, pid_tid.split(",")))
        pid = pid_tid[0]
        tid = pid_tid[1] if len(pid_tid) > 1 else 0
    print_pid_metrics_cache_enytry(pid, tid=tid, procfs_root=args.procfs_root)
