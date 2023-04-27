#! /usr/bin/env python3


import argparse
import os
import sys
import time

sys.path.append(
    os.path.dirname(os.path.dirname(os.path.normpath(os.path.abspath(__file__))))
)

from metrics_common_test import TestdataProcfsRoot
from pvmi.pid_list import get_pid_tid_list

pid_files = [
    "cgroup",
    "cmdline",
    "io",
    "stat",
    "status",
]


def read_pid_files(pid_tid_list: list, procfs_root: str = TestdataProcfsRoot):
    start = time.time()
    n_files, n_bytes = 0, 0
    for pid, tid in pid_tid_list:
        if tid == 0:
            pid_dir = os.path.join(procfs_root, str(pid))
        else:
            pid_dir = os.path.join(procfs_root, str(pid), "task", str(tid))

        for pid_file in pid_files:
            n_files += 1
            file_path = os.path.join(pid_dir, pid_file)
            try:
                with open(file_path, "rb") as f:
                    b = f.read()
                    n_bytes += len(b)
            except (FileNotFoundError, IOError) as e:
                print(f"{file_path}: {e}", file=sys.stderr)
    d_time = time.time() - start
    print(
        f"{n_files} file(s), {n_bytes} byte(s) read in {d_time:.06f} sec",
        file=sys.stderr,
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    args = parser.parse_args()

    pid_tid_list = get_pid_tid_list(procfs_root=args.procfs_root)
    read_pid_files(pid_tid_list, procfs_root=args.procfs_root)
