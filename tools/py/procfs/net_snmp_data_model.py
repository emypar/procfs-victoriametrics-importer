# /usr/bin/env python3

# handle /proc/net/snmp

import os
import time
from typing import Optional

from metrics_common_test import TestdataProcfsRoot

from .data_model import FixedLayoutDataModel, IndexRange

_path = "net/snmp"


def load_net_snmp_data_model(
    procfs_root: str = TestdataProcfsRoot,
    _use_ts_from_file: bool = True,
) -> Optional[FixedLayoutDataModel]:
    net_snmp_path = os.path.join(procfs_root, _path)
    ts = os.stat(net_snmp_path).st_mtime if _use_ts_from_file else time.time()
    net_snmp = FixedLayoutDataModel(
        _ts=ts,
        Values=list(),
        Names=list(),
        Groups=dict(),
    )
    with open(net_snmp_path, "rt") as f:
        start_index, n = 0, 0
        for line in f:
            n += 1
            words = line.split()
            if words[0][-1] != ":":
                raise RuntimeError(
                    f"{net_snmp_path}: line#{n}: {line!r}: missing colon"
                )
            fields = words[1:]
            if n & 1:
                # odd line: PROTO NAME...
                start_index = len(net_snmp.Names)
                proto = words[0][:-1]
                for name in fields:
                    net_snmp.Names.append(f"{proto}:{name}")
                net_snmp.Groups[proto] = [IndexRange(start_index, len(net_snmp.Names))]
            else:
                if proto != words[0][:-1]:
                    raise RuntimeError(
                        f"{net_snmp_path}: line#{n}: {line!r}: missing expected proto {proto!r}"
                    )
                net_snmp.Values.extend(fields)
                start_index += len(net_snmp.Values)
    assert len(net_snmp.Values) == len(net_snmp.Names)
    return net_snmp
