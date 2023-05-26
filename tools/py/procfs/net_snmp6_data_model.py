# /usr/bin/env python3

# handle /proc/net/snmp6

import os
import time
from typing import Optional

from metrics_common_test import TestdataProcfsRoot

from .data_model import FixedLayoutDataModel, IndexRange

_path = "net/snmp6"


def load_net_snmp6_data_model(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> Optional[FixedLayoutDataModel]:
    net_snmp6_path = os.path.join(procfs_root, _path)
    ts = os.stat(net_snmp6_path).st_mtime if _use_ts_from_file else time.time()
    net_snmp6 = FixedLayoutDataModel(
        _ts=ts,
        Values=list(),
        Names=list(),
        Groups=dict(),
    )
    with open(net_snmp6_path, "rt") as f:
        n = 0
        for line in f:
            n += 1
            name, value = line.split()
            idx = name.find("6")
            if idx == -1:
                raise RuntimeError(
                    f"{net_snmp6_path}: line#{n}: {line!r}: missing PROTO6 prefix"
                )
            proto = name[: idx + 1]
            net_snmp6.Values.append(value)
            net_snmp6.Names.append(name)
            end = len(net_snmp6.Values)
            if proto not in net_snmp6.Groups:
                net_snmp6.Groups[proto] = [IndexRange(end - 1, end)]
            elif net_snmp6.Groups[proto][-1].End == end - 1:
                net_snmp6.Groups[proto][-1].End = end
            else:
                net_snmp6.Groups[proto].append(IndexRange(end - 1, end))
    return net_snmp6
