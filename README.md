# procfs-victoriametrics-importer

An utility for importing granular Linux <a href="https://linux.die.net/man/5/proc" target="_blank">proc</a> stats into <a href="https://docs.victoriametrics.com/Cluster-VictoriaMetrics.html" target="_blank">VictoriaMetrics</a>

# Architecture

## Diagram

![architecture](docs/images/architecture.jpg)

## Components
### Scheduler
The scheduler is responsible for determining the next (the  nearest in time, that is) units of work that need to be done. A unit of work is represented by the pair **(metricsGenFn, metricsGenCtx)**, which is added to the **TODO Queue**.

### TODO Queue
A Golang channel storing the units of work, written by the **Scheduler** and read by worker goroutines. This is in order to support the parallelization of metrics generation.

### MetricsGenFn and metricsGenCtx

**MetricsGenFn** are functions capable of parsing [/proc](https://man7.org/linux/man-pages/man5/proc.5.html) information into [Prometheus exposition text format](https://github.com/prometheus/docs/blob/main/content/docs/instrumenting/exposition_formats.md#text-based-format). The functions operate in the context of **metricsGenCtx**, which a container of configuration, state and stats. Each metrics generator function has its specific context (not to be confused with the Golang Standard Library
 one).

The generated metrics are packed into buffers, until the latter reach ~ 64k in size (the last buffer of the scan may be shorter, of course). The buffers are written into the **Compressor Queue**

### Compressor Queue

A Golang channel with metrics holding buffers from all metrics generator functions, which are its writers. The readers are gzip compressor workers. This approach has 2 benefits:
* it supports the parallelization of compression
* it allows more efficient packing by consolidating metrics across all generator functions, compared to individual compression inside the latter.

## Compressor Workers

They perform gzip compression until either the compressed buffer reaches ~ 64k in size, or the partially compressed data becomes older than N seconds (time based flush, that is). Once a compressed buffer is ready to be sent, the compressor uses **SendFn**, the sender method of the **HTTP Sender Pool**, to ship it to an import end point.

## HTTP Sender Pool and SendFn

The **HTTP Sender Pool** holds information and state about all the configured VictoriaMetrics end points. The end points can be either healthy or unhealthy. If a send operation fails, the used end point is moved to the unhealthy list. The latter is periodically checked by health checkers and end points that pass the check are moved back to the healthy list. **SendFn** is a method of the **HTTP Sender Pool** and it works with the latter to maintain the healthy / unhealthy lists. The **Compressor Workers** that actually invoke **SendFn** are unaware of these details, they are simply informed that the compressed buffer was successfully sent or that it was discarded (after a number of attempts). The healthy end points are used in a round robin fashion to spread the load across all of the VictoriaMetrics import end points.

## Bandwidth Control

The **Bandwidth Control** implements a credit based mechanism to ensure that the egress traffic across all **SendFn** invocations does not exceed a certain limit. This is useful in smoothing bursts when all metrics are generated at the same time, e.g. at start.

