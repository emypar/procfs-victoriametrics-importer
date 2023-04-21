// Command line args for pid_metrics

package pvmi

import (
	"flag"
	"fmt"
	"time"
)

const (
	DEFAULT_PID_METRICS_NUM_GENERATORS          = -1        // i.e. use available CPU count.
	MAX_NUM_PID_METRICS_GENERATORS              = 4         // ceiling when based on CPU count
	DEFAULT_PID_METRICS_SCAN_INTERVAL           = 1.        // seconds
	DEFAULT_PID_METRICS_FULL_METRICS_INTERVAL   = 15.       // seconds
	DEFAULT_PID_METRICS_ACTIVE_THRESHOLD_PCT    = 1.        // %CPU
	DEFAULT_PID_METRICS_PID_TID_SELECTION       = "pid+tid" // "pid"|"tid"|"pid+tid"
	DEFAULT_PID_METRICS_PID_LIST_VALID_INTERVAL = -1.       // i.e. use 1/2 * scan interval

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

func GetPidMetricsGeneratorsCountFromArgs() int {
	numPidMetricsGenerators := *NumPidMetricsGeneratorsArg
	if numPidMetricsGenerators < 0 {
		numPidMetricsGenerators = AvailableCpusCount
		if numPidMetricsGenerators > MAX_NUM_PID_METRICS_GENERATORS {
			numPidMetricsGenerators = MAX_NUM_PID_METRICS_GENERATORS
		}
	}
	return numPidMetricsGenerators
}

func BuildPidMetricsCtxFromArgs() ([]*PidMetricsContext, error) {
	if *PidMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*PidMetricsScanIntervalArg * float64(time.Second))

	pidListFlags := uint32(0)
	pidTidSelection := *PidMetricsPidTidSelectionArg
	switch pidTidSelection {
	case "pid":
		pidListFlags = PID_LIST_CACHE_PID_ENABLED_FLAG
	case "tid":
		pidListFlags = PID_LIST_CACHE_TID_ENABLED_FLAG
	case "pid+tid":
		pidListFlags = PID_LIST_CACHE_PID_ENABLED_FLAG | PID_LIST_CACHE_TID_ENABLED_FLAG
	default:
		return nil, fmt.Errorf(
			"%s: invalid process/thread selection, not one of pid, tid or pid+tid",
			pidTidSelection,
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
		GlobalProcfsRoot,
		pidListFlags,
	)

	fullMetricsInterval := time.Duration(*PidMetricsFullMetricsIntervalArg * float64(time.Second))
	fullMetricsFactor := int(fullMetricsInterval / interval)
	// Convert active threshold to ticks:
	// %CPU = delta ticks * ClktckSec / scan interval * 100
	activeThreshold := uint((*PidMetricsActiveThresholdPctArg / 100.) / ClktckSec)

	// Ensure common metrics pre-requisites:
	err := SetCommonMetricsPreRequisitesFromArgs()
	if err != nil {
		return nil, err
	}

	PidMetricsLog.Infof("pid_metrics: interval=%s", interval)
	PidMetricsLog.Infof("pid_metrics: pidTidSelection=%s", pidTidSelection)
	PidMetricsLog.Infof("pid_metrics: pidListValidInterval=%s", pidListValidInterval)
	PidMetricsLog.Infof("pid_metrics: procfsRoot=%s", GlobalProcfsRoot)
	PidMetricsLog.Infof("pid_metrics: fullMetricsFactor=%d", fullMetricsFactor)
	PidMetricsLog.Infof("pid_metrics: activeThreshold=%d", activeThreshold)
	PidMetricsLog.Infof("pid_metrics: hostname=%s", GlobalMetricsHostname)
	PidMetricsLog.Infof("pid_metrics: job=%s", GlobalMetricsJob)
	PidMetricsLog.Infof("pid_metrics: clktckSec=%f", ClktckSec)

	PidMetricsLog.Infof("pid_metrics: numGenerators=%d", numGenerators)

	pidMetricsContextList := make([]*PidMetricsContext, numGenerators)
	for pidListPart := 0; pidListPart < numGenerators; pidListPart++ {
		pidMetricsContext, err := NewPidMetricsContext(
			interval,
			GlobalProcfsRoot,
			pidListPart,
			fullMetricsFactor,
			activeThreshold,
			// needed for testing:
			GlobalMetricsHostname,
			GlobalMetricsJob,
			ClktckSec,
			// will be set to default values:
			nil, // timeNow TimeNowFn,
			nil, // wChan chan *bytes.Buffer,
			nil, // getPids GetPidsFn,
			nil, // bufPool *BufferPool,
		)
		if err != nil {
			return nil, err
		}
		pidMetricsContextList[pidListPart] = pidMetricsContext
	}
	return pidMetricsContextList, nil
}

var GlobalPidMetricsCtxList []*PidMetricsContext

func StartPidMetricsFromArgs() error {
	pidMetricsContextList, err := BuildPidMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if pidMetricsContextList == nil {
		PidMetricsLog.Warn("PID metrics collection disabled")
		return nil
	}

	GlobalPidMetricsCtxList = pidMetricsContextList
	for _, pidMetricsCtx := range GlobalPidMetricsCtxList {
		PidMetricsLog.Infof(
			"Start PID metrics generator: interval=%s, partition=%d",
			pidMetricsCtx.interval,
			pidMetricsCtx.pidListPart,
		)
		GlobalSchedulerContext.Add(GenerateAllPidMetrics, MetricsGenContext(pidMetricsCtx))
	}

	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartPidMetricsFromArgs)
}
