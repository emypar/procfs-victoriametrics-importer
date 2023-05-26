# /usr/bin/env python3

#  https://github.com/prometheus/procfs like module


import dataclasses
from typing import Dict, List, Optional

from .common import ProcfsStructBase, StructBase


@dataclasses.dataclass
class IndexRange(StructBase):
    Start: int = 0
    End: int = 0


@dataclasses.dataclass
class FixedLayoutDataModel(ProcfsStructBase):
    Values: Optional[List[str]] = None
    Names: Optional[List[str]] = None
    Groups: Optional[Dict[str, List[IndexRange]]] = None
