#! /usr/bin/env python3

# support for pvmi/pid_list.go testing

import os

from metrics_common_test import TestdataProcfsRoot, TestdataTestCasesDir
from tools_common import rel_path_to_file, save_to_json_file

from .pid_list import get_pid_tid_list

# Match pvmi/pid_list_test.go:
PID_LIST_TEST_CASE_FILE_NAME = "pid_list_test_case.json"


def generate_test_case_files(
    procfs_root: str = TestdataProcfsRoot,
    test_case_dir: str = TestdataTestCasesDir,
):
    test_case_json_file = os.path.join(test_case_dir, PID_LIST_TEST_CASE_FILE_NAME)

    # Match struct PidListTestCase fields from pvmi/pid_list_test.go:
    pid_list_test_case = {
        "ProcfsRoot": rel_path_to_file(procfs_root),
        "PidTidList": get_pid_tid_list(procfs_root),
    }
    save_to_json_file(pid_list_test_case, test_case_json_file)
