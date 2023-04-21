#! /usr/bin/env python3

# support for pvmi/proc_stat_metrics.go testing

from typing import Optional

import procfs
from tools_common import sanitize_label_value

from .common import Metric, register_metrics_fn

metrics_fn_map = {}


