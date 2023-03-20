#! /usr/bin/env python3


import argparse
import os
import pprint
import sys

sys.path.append(
    os.path.dirname(os.path.dirname(os.path.normpath(os.path.abspath(__file__))))
)

from metrics_common_test import TestdataProcfsRoot
from pvmi.pid_list import get_pid_tid_list

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    args = parser.parse_args()

    pprint.pprint(get_pid_tid_list(procfs_root=args.procfs_root))
