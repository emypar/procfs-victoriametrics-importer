#! /usr/bin/env python3

# support for pvmi/pid_list.go testing

import os
from typing import List, Optional

from metrics_common_test import TestdataProcfsRoot


def get_pid_tid_list(procfs_root: str = TestdataProcfsRoot) -> List[List[int]]:
    pid_tid_list = []

    def _to_int(v: str) -> Optional[int]:
        try:
            return int(v)
        except (ValueError, TypeError):
            return None

    for p in os.listdir(procfs_root):
        pid = _to_int(p)
        if pid is None:
            continue
        pid_tid_list.append((pid, 0))
        task_dir = os.path.join(procfs_root, p, "task")
        if not os.path.isdir(task_dir):
            continue
        for t in os.listdir(task_dir):
            tid = _to_int(t)
            if tid is None:
                continue
            pid_tid_list.append((pid, tid))
    return sorted(pid_tid_list)
