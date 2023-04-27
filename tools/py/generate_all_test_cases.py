#! /usr/bin/env python3

import argparse

from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from pvmi import (
    pid_list_test,
    proc_net_dev_metrics_test,
    proc_pid_metrics_test,
    proc_stat_metrics_test,
)

gtc_map = {
    m.__name__.split(".")[-1].replace("_test", ""): m.generate_test_case_files
    for m in [
        pid_list_test,
        proc_pid_metrics_test,
        proc_stat_metrics_test,
        proc_net_dev_metrics_test,
    ]
}


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    parser.add_argument("-t", "--test-case-dir", default=TestdataTestCasesDir)
    parser.add_argument(
        "-s",
        "--select",
        help=f"""
    Selector, comma separated list from {sorted(gtc_map)}
    """,
    )
    args = parser.parse_args()
    if args.select is not None:
        gtc_fn_list = [gtc_map[selector] for selector in args.select.split(",")]
    else:
        gtc_fn_list = gtc_map.values()
    for gtc_fn in gtc_fn_list:
        gtc_fn(procfs_root=args.procfs_root, test_case_dir=args.test_case_dir)
