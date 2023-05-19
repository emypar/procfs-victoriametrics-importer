# Imported Stats

## Metrics Naming Convention

proc_*filepath*\_*stat* for non `/proc/PID` stats and proc_pid_*filepath*\_*stat* for `/proc/PID` stats.

Examples:

`proc_net_dev_rx_packets` for the `Receive packets` column extracted from `/proc/net/dev` file.

`proc_pid_stat_utime` for the `utime` row extracted from `/proc/PID/stat` file.

## Common Labels

All metrics will have the following labels:

| Label | Default Type | Command Arg |
| ----- | ------------- | ----------- |
| `hostname`| os hostname | `--metrics-hostname`<br>`-metrics-use-short-hostname` |
| `job` | `pvmi` | `-metrics-job` |


## Handling proc_pid_\* Metrics

Every metric will have 2 mandatory labels, `pid=PID` and `starttime=TICKS` to disambiguate for the case where the same PID was re-used by a later invocation (starttime read from `/proc/PID/stat` file).

Thread metrics, i.e. based on `/proc/PID/task/TID` files, will have 2 additional labels: `tid=TID` and `t_starttime=TICKS` to disambiguate for the case where the same PID was re-used by a later invocation (starttime read from `/proc/PID/task/TID/stat` file).

## Handling Certain Categorical Metrics

Certain categorical values, e.g. `proc_pid_stat_state` may have many categories, so the canonical approach of having N metrics with `label=CATEGORY`, one per category, with the associated w/ value 1 for the currently in-use category and 0 for the rest, may be inefficient. To make this more efficient, only the metric w/ the category currently in-use is being generated at every scan, while the others are generated only when they transition from having been in-use (associated value 1 -> 0).

Such metrics will be referred as pseudo-categorical.

Example: `proc_pid_stat_state` transitioning from state `R` to `S` at `t2`:

    Time        Generated metrics
    t1:         proc_pid_stat_state{state=R}=1
    t2:         proc_pid_stat_state{state=R}=0
                proc_pid_stat_state{state=S}=1
    t3:         proc_pid_stat_state{state=S}=1



## Handling Large/High Precision Values

Certain metrics have values that would exceed the 15 (15.95) precision of `float64`, e.g. `uint64`. Such values will be split in two metrics, `..._low32` and `..._high32` where `low32` == `value & 0xffffffff` and `high32` = `value >> 32`. This is useful for deltas or rates where the operation is applied 1st to each of the 2 halves which are then combined. e.g.: `delta = delta(high32) * 4294967296 + delta(low32)`, with the underlying assumption that the delta is fairly small. This is how the byte count is handled for interfaces.

## Reducing The Number Of Data Points

### Delta / Full Refresh

In order to reduce the traffic between the importer and the import endpoints, only the metrics whose values have changed from the previous scan are being generated and sent. That requires that queries be made using the [last_over_time(METRIC[RANGE_INTERVAL])](https://prometheus.io/docs/prometheus/latest/querying/functions/#aggregation_over_time) function and to make the range interval predictable, all metrics have a guaranteed full refresh interval, when a metric is being generated regardless of its lack of change from the previous scan.

Each metrics generator is configured with 2 intervals: _scan_ and _full\_refresh_. The ratio N = _full\_refresh_ / _scan_ is called _full\_refresh\_factor_ and it means that a specific metric will be generated every N scans.

Ideally the full refresh cycles should be spread evenly across all metrics provided by a specific generator, leading to the following approach:

* each metric is associated with a _refresh\_group\_num_, 0..N-1. This is a cyclic number assigned first time when the object to which the metric belongs is being discovered and it is incremented modulo N after each use
* each scan has a _refresh\_cycle\_num_, 0..N-1. This is a cyclic counter incremented modulo N after each scan
* metrics that have _refresh\_group\_num_ == _refresh\_cycle\_num_ will be generated part of the full refresh

Since the association of a particular metric with a  _refresh\_group\_num_ is unchanged for the lifetime of the metric, the metric is guaranteed to be generated at least every Nth scan.

Since the _refresh\_group\_num_, assigned at object creation, is incremented modulo N afterwards, this will lead to the spread of the full refresh for the metrics associated with those objects over N cycles.

**Note:** The delta approach can be disabled by setting the _full\_refresh_ interval to 0.

e.g.

Let's consider the case of the `proc_pid_metrics` generator w/ N, the full refresh factor, = 15 and with the  _new\_refresh\_group\_num_, the next _refresh\_group\_num_ to be assigned, = 12. In the current scan 5 new PIDs are being discovered: 1001, 1002, 1003, 1004 and 1005. The _refresh\_group\_num_ assignment will be as following:
| PID | _refresh\_group\_num_ |
| ---: | ---------------------: |
| 1001 | 12 |
| 1002 | 13 |
| 1003 | 14 |
| 1004 | 0 |
| 1005 | 1 |

Thus each of the new PID will have a full refresh in a **different** scan cycle.

### Active Processes/Threads

In addition to the delta approach, process/thread metrics use the concept of active process to further reduce the number of metrics. PIDs/TIDs are classified into active/inactive based upon %CPU since the previous scan >= _active\_threshold_. Memory and I/O stats are parsed and the associated metrics are generated only for active PIDs/TIDs.

**Note** The active process check can be disabled by setting _active\_threshold_ to 0.



## Metrics

### proc_buddyinfo_count

Source: `/proc/buddyinfo`

Parser: [buddyinfo.go](https://github.com/prometheus/procfs/blob/master/buddyinfo.go)

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_buddyinfo_count | equinode=_node_<br>zone=_zone_<br>index=_index_ | Counter | |


### 


### proc_pid_cgroup

Source: `/proc/PID/cgroup` or `/proc/PID/task/TID/cgroup`

Parser: [proc_cgroup.go](https://github.com/prometheus/procfs/blob/master/proc_cgroup.go)

Reference: [man cgroups](https://man7.org/linux/man-pages/man7/cgroups.7.html), see `/proc/[pid]/cgroup`

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_pid_cgroup | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_<br><br>hierarchy_id=_id_<br>controller=_controller_<br> path=_path_ | 0/1 | pseudo-categorical |



### proc_pid_cmdline

Source: `/proc/PID/cmdline` or `/proc/PID/task/TID/cmdline`


| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_pid_cmdline | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_<br><br>cmdline=_cmdline_ | 0/1 | The command line string, with `\0` (null) replaced by `' '` (space).<br>pseudo-categorical |


### proc_pid_io_\*

Source: `/proc/PID/io` or `/proc/PID/task/TID/io`

Parser: [proc_io.go](https://github.com/prometheus/procfs/blob/master/proc_io.go)

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_pid_io_rcar | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_wcar | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_syscr | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_syscw | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_readbytes | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_writebytes | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_io_cancelled_writebytes | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |

### proc_pid_stat_\*

Source: `/proc/PID/stat` or `/proc/PID/task/TID/stat`

Parser: [proc_stat.go](https://github.com/prometheus/procfs/blob/master/proc_stat.go)


| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_pid_stat_state | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_<br><br>state=`R\|S\|D\|...` | 0/1 | pseudo-categorical |
| proc_pid_stat_info | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_<br><br>comm=_comm_<br>ppid=_ppid_<br>pgrp=_pgrp_<br>session=_session_<br>tty_nr=*tty_nr*<br>tpgid=_tpgid_<br>flags=*pf_flags*<br>priority=_priority_<br>nice=_nice_<br>rt_priority=*rt_priority*<br>policy=_policy_ | 0/1 | Information associated w/ a process/thread that may change but it is unlikely to do so for the lifespan of a process<br><br>pseudo-categorical |
| proc_pid_stat_minflt | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_stat_cminflt | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_stat_majflt | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_stat_cmajflt | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_stat_utime_seconds | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | The value read from file divided by `sysconf(_SC_CLK_TCK)` |
| proc_pid_stat_stime_seconds | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | The value read from file divided by `sysconf(_SC_CLK_TCK)` |
| proc_pid_stat_cutime_seconds | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | The value read from file divided by `sysconf(_SC_CLK_TCK)` |
| proc_pid_stat_cstime_seconds | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | The value read from file divided by `sysconf(_SC_CLK_TCK)` |
| proc_pid_stat_utime_pct | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | The %CPU in user space |
| proc_pid_stat_stime_pct | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | The %CPU in system space  |
| proc_pid_stat_cpu_time_pct | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | The %CPU in user+system space  |
| proc_pid_stat_vsize | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | |
| proc_pid_stat_rss | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | |
| proc_pid_stat_rsslim | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | |
| proc_pid_stat_cpu | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Gauge | |


### proc_pid_status_\*

Source: `/proc/PID/status` or `/proc/PID/task/TID/status`

Parser: [proc_status.go](https://github.com/prometheus/procfs/blob/master/proc_status.go)


**Note:** Memory metrics are generated **only** for processes (PIDs) not threads (TIDs) since all threads in a process share the memory.

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_pid_status_info | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_<br><br>name=_name_<br>tgid=_tgid_<br>real_uid=_uid_<br>effective_uid=_uid_<br>saved_uid=_uid_<br>filesystem_uid=_uid_<br>real_gid=_gid_<br>effective_gid=_gid_<br>saved_gid=_gid_<br>filesystem_gid=_gid_ | 0/1 | Information associated w/ a process/thread that may change but it is unlikely to do so for the lifespan of a process<br><br>pseudo-categorical |
| proc_pid_status_vm_peak | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_size | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_lck | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_pin | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_hwm | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_rss | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_rss_anon | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_rss_file | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_rss_shmem | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_data | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_stk | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_exe | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_lib | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_pte | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_pmd | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_vm_swap | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_hugetbl_pages | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_ | Gauge | |
| proc_pid_status_voluntary_ctxt_switches | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |
| proc_pid_status_nonvoluntary_ctxt_switches | hostname=_hostname_<br>job=_job_<br><br>pid=_pid_<br>starttime=_ticks_<br>tid=_tid_<br>t_starttime=_ticks_ | Counter | |

**TODO**: add support for `cpuset` amd `memset` to the parser.

### proc_stat_\*

Source: `/proc/stat`

Parser: [stat.go](https://github.com/prometheus/procfs/blob/master/stat.go)

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_stat_cpu_time_seconds | hostname=_hostname_<br>job=_job_<br><br>cpu=_cpu_\|all<br>type=user\|nice\|system\|idle\|iowait\|irq\|softirq\|steal\|guest\|guest_nice |  | The value read from file divided by `sysconf(_SC_CLK_TCK) |
| proc_stat_cpu_time_pct | hostname=_hostname_<br>job=_job_<br><br>cpu=_cpu_\|all<br>type=user\|nice\|system\|idle\|iowait\|irq\|softirq\|steal\|guest\|guest_nice |  |  |
| proc_stat_boot_time | hostname=_hostname_<br>job=_job_<br> | |
| proc_stat_irq_total_count | hostname=_hostname_<br>job=_job_<br> | Counter | |
| proc_stat_softirq_total_count | hostname=_hostname_<br>job=_job_<br> | Counter | |
| proc_stat_context_switches_count | hostname=_hostname_<br>job=_job_<br> | Counter | |
| proc_stat_process_created_count | hostname=_hostname_<br>job=_job_<br> | Counter | |
| proc_stat_process_running_count | hostname=_hostname_<br>job=_job_<br> | Gauge | |
| proc_stat_process_blocked_count | hostname=_hostname_<br>job=_job_<br> | Gauge | |

### proc_net_dev_\*

Source: `/proc/net/dev`

Parser: [net_dev.go](https://github.com/prometheus/procfs/blob/master/net_dev.go)

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_net_dev_bytes_total_high32<br>proc_net_dev_bytes_total_low32 | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | The 2 halves of the very large counter, (`uint64`) |
| proc_net_dev_bps | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Gauge | bits/sec |
| proc_net_dev_packets_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | |
| proc_net_dev_errors_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | |
| proc_net_dev_dropped_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | |
| proc_net_dev_fifo_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | |
| proc_net_dev_frame_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=rx | Counter | |
| proc_net_dev_compressed_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=`rx`\|`tx` | Counter | |
| proc_net_dev_multicast_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=rx | Counter | |
| proc_net_dev_collisions_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=tx | Counter | |
| proc_net_dev_carrier_total | hostname=_hostname_<br>job=_job_<br><br>device=_name_<br>side=tx | Counter | |

### proc_interrupts_*

Source: `/proc/interrupts`

Parser: [proc_interrupts.go](https://github.com/prometheus/procfs/blob/master/proc_interrupts.go)


| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_interrupts_total | hostname=_hostname_<br>job=_job_<br><br>interrupt=_num_\|_name_<br>cpu=_cpu_ | Counter | |
| proc_interrupts_info | hostname=_hostname_<br>job=_job_<br><br>interrupt=_num_\|_name_<br>devices=_devices_<br>info=_info_ | 0\|1 | Pseudo-categorical |

### proc_softirqs_*

Source: `/proc/interrupts`

Parser: [proc_interrupts.go](https://github.com/prometheus/procfs/blob/master/softirqs.go)

| Metric | Labels | Type | Obs |
| ------ | ------ | ----- | --- |
| proc_softirqs_total | hostname=_hostname_<br>job=_job_<br><br>interrupt=_name_<br>cpu=_cpu_ | Counter | |
