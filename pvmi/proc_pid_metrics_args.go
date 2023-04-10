// Command line args for pid_metrics

package pvmi

import (
	"flag"
	"fmt"
	"time"
)

const (
	DEFAULT_PID_METRICS_NUM_GENERATORS          = -1    // i.e. use available CPU count.
	MAX_NUM_PID_METRICS_GENERATORS              = 4     // ceiling when based on CPU count
	DEFAULT_PID_METRICS_SCAN_INTERVAL           = 1.    // seconds
	DEFAULT_PID_METRICS_FULL_METRICS_INTERVAL   = 15.   // seconds
	DEFAULT_PID_METRICS_ACTIVE_THRESHOLD_PCT    = 1.    // %CPU
	DEFAULT_PID_METRICS_PID_TID_SELECTION       = "tid" // "pid"|"tid"|"pid+tid"
	DEFAULT_PID_METRICS_PID_LIST_VALID_INTERVAL = -1.   // i.e. use 1/2 * scan interval

)

var PidMetricsScanIntervalArg = flag.Float64(
	"pid-metrics-scan-interval",
	DEFAULT_PID_METRICS_SCAN_INTERVAL,
	`Pid metrics interval in seconds, use 0 to disable.`,
)

var NumPidMetricsGeneratorsArg = flag.Int(
	"pid-metrics-num-generators",
	DEFAULT_PID_METRICS_NUM_GENERATORS,
	FormatFlagUsage(fmt.Sprintf(`
	The number of PID metrics generators that run in parallel. If -1
	then it is based on the number of available CPUs capped to %d.
	`, MAX_NUM_PID_METRICS_GENERATORS)),
)

var PidMetricsFullMetricsIntervalArg = flag.Float64(
	"pid-metrics-full-metrics-interval",
	DEFAULT_PID_METRICS_FULL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often to generate full metrics, in seconds; normally only the metrics
	whose value has changed from the previous scan are generated, but every
	so often the entire set is generated to prevent queries from having to go
	too much back in time to find the last value. Use 0 to generate full
	metrics at every scan.
	`),
)

var PidMetricsActiveThresholdPctArg = flag.Float64(
	"pid-metrics-active-threshold-pct",
	DEFAULT_PID_METRICS_ACTIVE_THRESHOLD_PCT,
	FormatFlagUsage(`
	The threshold for active PID's, in percents. Read the full set of stats
	only for PIDs that have %CPU >= threshold since the last scan, AKA active
	PIDs. If this is a full scan cycle then all stats are read. Use 0 to read
	full stats at every scan.
	`),
)

var PidMetricsPidTidSelectionArg = flag.String(
	"pid-metrics-pid-tid-selection",
	DEFAULT_PID_METRICS_PID_TID_SELECTION,
	FormatFlagUsage(`
	Whether to collect metrics for processes (pid), threads (tid) or both
	(pid+tid). 
	`),
)

var PidMetricsPidListValidIntervalArg = flag.Float64(
	"pid-metrics-pid-list-valid-interval",
	DEFAULT_PID_METRICS_PID_LIST_VALID_INTERVAL,
	FormatFlagUsage(`
	The interval, in seconds, to cache the directory list of /proc/PID and/or
	/proc/PID/task/TID, useful to avoid multiple scans when the list is
	partitioned across multiple metrics generators. If -1 then the interval
	is 1/2 x pid metrics scan interval.
	`),
)

func BuildPidMetricsCtxFromArgs() ([]*PidMetricsContext, error) {
	if *PidMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*PidMetricsScanIntervalArg * float64(time.Second))

	pidListFlags := uint32(0)
	switch *PidMetricsPidTidSelectionArg {
	case "pid":
		pidListFlags = PID_LIST_CACHE_PID_ENABLED_FLAG
	case "tid":
		pidListFlags = PID_LIST_CACHE_TID_ENABLED_FLAG
	case "pid+tid":
		pidListFlags = PID_LIST_CACHE_PID_ENABLED_FLAG | PID_LIST_CACHE_TID_ENABLED_FLAG
	default:
		return nil, fmt.Errorf(
			"%s: invalid process/thread selection, not one of pid, tid or pid+tid",
			*PidMetricsPidTidSelectionArg,
		)
	}
	pidListValidInterval := time.Duration(*PidMetricsPidListValidIntervalArg * float64(time.Second))
	if pidListValidInterval < 0 {
		pidListValidInterval = interval / 2
	}
	numGenerators := *NumPidMetricsGeneratorsArg
	if numGenerators <= 0 {
		numGenerators = AvailableCpusCount
		if numGenerators > MAX_NUM_PID_METRICS_GENERATORS {
			numGenerators = MAX_NUM_PID_METRICS_GENERATORS
		}
	}

	GlobalPidListCache = NewPidListCache(
		numGenerators,
		pidListValidInterval,
		*MetricsProcfsRootArg,
		pidListFlags,
	)

	fullMetricsInterval := time.Duration(*PidMetricsFullMetricsIntervalArg * float64(time.Second))
	fullMetricsFactor := int(fullMetricsInterval / interval)
	// Convert active threshold to ticks:
	// %CPU = delta ticks * ClktckSec / scan interval * 100
	activeThreshold := uint((*PidMetricsActiveThresholdPctArg / 100.) / ClktckSec)

	pidMetricsContextList := make([]*PidMetricsContext, numGenerators)
	for pidListPart := 0; pidListPart < numGenerators; pidListPart++ {
		pidMetricsContext, err := NewPidMetricsContextFromArgs(
			interval,
			*MetricsProcfsRootArg,
			pidListPart,
			fullMetricsFactor,
			activeThreshold,
		)
		if err != nil {
			return nil, err
		}
		pidMetricsContextList[pidListPart] = pidMetricsContext
	}
	return pidMetricsContextList, nil
}
