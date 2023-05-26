#! /usr/bin/env python3

# tools for pvmi test support
import argparse
from typing import Dict, Optional

import procfs
from metrics_common_test import TestdataProcfsRoot
from pvmi.common import make_metric_name


def build_fldm_metric_name_map(
    fldm: procfs.FixedLayoutDataModel,
    prefix: Optional[str] = None,
    suffix: Optional[str] = None,
    exceptions: Optional[Dict[str, str]] = None,
    suffix_exceptions: Optional[Dict[str, str]] = None,
) -> Dict[str, str]:
    name_map = {}
    if exceptions:
        name_map.update(exceptions)
    for name in fldm.Names:
        if name not in name_map:
            if suffix_exceptions:
                use_suffix = suffix_exceptions.get(name, suffix)
            else:
                use_suffix = suffix
            name_map[name] = make_metric_name(name, prefix=prefix, suffix=use_suffix)
    return name_map


def metric_name_map_asstr(
    name_map: Dict[str, str],
    go: bool = False,
) -> str:
    """Build common py/go representation"""
    map_str = "{\n"
    indent = "\t" if go else " " * 2
    for name in sorted(name_map):
        map_str += indent + f'"{name}": "{name_map[name]}",\n'
    map_str += "}\n"
    return map_str


def print_net_snmp_metric_name_map(
    procfs_root: str = TestdataProcfsRoot,
    go: bool = False,
) -> str:
    net_snmp = procfs.load_net_snmp_data_model(procfs_root=procfs_root)
    name_map = build_fldm_metric_name_map(
        net_snmp,
        prefix="net_snmp",
        suffix="total",
    )
    if go:
        print("var NetSnmpMetricNameMap map[string]string = ", end="")
    else:
        print("net_snmp_metric_name_map = ", end="")
    print(metric_name_map_asstr(name_map, go=go))


def print_net_snmp6_metric_name_map(
    procfs_root: str = TestdataProcfsRoot,
    go: bool = False,
) -> str:
    net_snmp6 = procfs.load_net_snmp6_data_model(procfs_root=procfs_root)
    name_map = build_fldm_metric_name_map(
        net_snmp6,
        prefix="net_snmp6",
        suffix="total",
    )
    if go:
        print("var NetSnmp6MetricNameMap map[string]string = ", end="")
    else:
        print("net_snmp6_metric_name_map = ", end="")
    print(metric_name_map_asstr(name_map, go=go))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("-r", "--procfs-root", default=TestdataProcfsRoot)
    parser.add_argument("--go", action="store_true")

    args = parser.parse_args()

    print_net_snmp_metric_name_map(procfs_root=args.procfs_root, go=args.go)

    print_net_snmp6_metric_name_map(procfs_root=args.procfs_root, go=args.go)
