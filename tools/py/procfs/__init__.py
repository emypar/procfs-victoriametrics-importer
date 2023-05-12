# /usr/bin/env python3

#  https://github.com/prometheus/procfs like module

import os
import sys
from typing import Union

tools_dir = os.path.dirname(
    os.path.normpath(os.path.abspath(os.path.dirname(__file__)))
)
sys.path.extend(tools_dir)


from .common import ProcfsStructFieldSpec, ProcfsStructVal
from .interrupts import Interrupt, Interrupts, load_interrupts
from .net_dev import NetDev, NetDevLine, load_net_dev
from .proc_cgroup import ProcPidCgroups, load_proc_pid_cgroups
from .proc_cmdline import ProcPidCmdline, load_proc_pid_cmdline
from .proc_io import ProcIO, load_proc_pid_io
from .proc_stat import ProcStat, load_proc_pid_stat
from .proc_status import ProcStatus, load_proc_pid_status
from .stat import Stat, load_stat
from .stat2 import Stat2, load_stat2

ProcfsStructType = Union[
    NetDev,
    Interrupts,
    ProcStat,
    ProcStatus,
    ProcIO,
    ProcPidCgroups,
    ProcPidCmdline,
    Stat,
    Stat2,
]
