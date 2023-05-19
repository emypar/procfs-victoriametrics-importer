# /usr/bin/env python3

# handle /proc/softirqs
# reference: https://github.com/prometheus/procfs/blob/master/softirqs.go

import dataclasses
import os
import time
from typing import List

from metrics_common_test import TestdataProcfsRoot

from .common import ProcfsStructBase


@dataclasses.dataclass
class Softirqs(ProcfsStructBase):
    Hi: List[int] = dataclasses.field(default_factory=list)
    Timer: List[int] = dataclasses.field(default_factory=list)
    NetTx: List[int] = dataclasses.field(default_factory=list)
    NetRx: List[int] = dataclasses.field(default_factory=list)
    Block: List[int] = dataclasses.field(default_factory=list)
    IRQPoll: List[int] = dataclasses.field(default_factory=list)
    Tasklet: List[int] = dataclasses.field(default_factory=list)
    Sched: List[int] = dataclasses.field(default_factory=list)
    HRTimer: List[int] = dataclasses.field(default_factory=list)
    RCU: List[int] = dataclasses.field(default_factory=list)


file_word_to_field_map = {
    "HI:": "Hi",
    "TIMER:": "Timer",
    "NET_TX:": "NetTx",
    "NET_RX:": "NetRx",
    "BLOCK:": "Block",
    "IRQ_POLL:": "IRQPoll",
    "TASKLET:": "Tasklet",
    "SCHED:": "Sched",
    "HRTIMER:": "HRTimer",
    "RCU:": "RCU",
}


def load_softirqs(
    procfs_root: str = TestdataProcfsRoot,
    keep_num_cpus: int = 0,
    _use_ts_from_file: bool = True,
) -> Softirqs:
    softirqs_path = os.path.join(procfs_root, "softirqs")
    ts = os.stat(softirqs_path).st_mtime if _use_ts_from_file else time.time()
    softirqs = Softirqs(_ts=ts)
    with open(softirqs_path, "rt") as f:
        for line in f:
            words = line.split()
            if len(words) < 2:
                continue
            field = file_word_to_field_map.get(words[0])
            if field is None:
                continue
            setattr(
                softirqs,
                field,
                [
                    int(n)
                    for n in (
                        words[1 : keep_num_cpus + 1] if keep_num_cpus > 0 else words[1:]
                    )
                ],
            )
    return softirqs
