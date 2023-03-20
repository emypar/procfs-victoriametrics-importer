#! /usr/bin/env python3

import argparse

from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from pvmi import pid_list_test, proc_pid_metrics_test

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    parser.add_argument("-t", "--test-case-dir", default=TestdataTestCasesDir)
    args = parser.parse_args()
    for gtc_fn in [
        pid_list_test.generate_test_case_files,
        proc_pid_metrics_test.generate_test_case_files,
    ]:
        gtc_fn(procfs_root=args.procfs_root, test_case_dir=args.test_case_dir)
