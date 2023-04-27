#! /usr/bin/env python3

# tools for pvmi test support

import os

py_tools_dir = os.path.normpath(os.path.abspath(os.path.dirname(__file__)))


# The following should match pvmi/metrics_common_test.go:
PVMI_TOP_DIR = os.path.dirname(
    os.path.dirname(py_tools_dir)
)  # lands into the same location as the .go one, the latter is relative.
TestdataProcfsRoot = os.path.join(PVMI_TOP_DIR, "testdata/proc")
TestdataTestCasesDir = os.path.join(PVMI_TOP_DIR, "testdata/testcases")
TestClktckSec = 0.01
TestHostname = "test-host"
TestJob = "testpvmi"
