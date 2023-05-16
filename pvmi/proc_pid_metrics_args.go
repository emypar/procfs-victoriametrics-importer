// Command line args for pid_metrics

package pvmi

import (
	"flag"
	"fmt"
	"time"
)

const (
	DEFAULT_PID_METRICS_NUM_GENERATORS          = -1  // i.e. use available CPU count.
	MAX_NUM_PID_METRICS_GENERATORS              = 4   // ceiling when based on CPU count
	DEFAULT_PID_METRICS_SCAN_INTERVAL           = 1.  // seconds
	DEFAULT_PID_METRICS_FULL_METRICS_INTERVAL   = 15. // seconds
	DEFAULT_PID_METRICS_ACTIVE_THRESHOLD_PCT    = 1.  // %CPU
	DEFAULT_PID_METRICS_PID_LIST_VALID_INTERVAL = -1. // i.e. use 1/2 * scan interval

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

var PidMetricsDisableTidArg = flag.Bool(
	"pid-metrics-disable-tid",
	false,
	FormatFlagUsage(`
	Disable metrics collection for threads (tid). 
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

	pidListFlags := PID_LIST_CACHE_PID_ENABLED_FLAG
	if !*PidMetricsDisableTidArg {
		pidListFlags |= PID_LIST_CACHE_TID_ENABLED_FLAG
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

	PidMetricsLog.Infof("proc_pid metrics: interval=%s", interval)
	PidMetricsLog.Infof("proc_pid metrics: disableTid=%v", *PidMetricsDisableTidArg)
	PidMetricsLog.Infof("proc_pid metrics: pidListValidInterval=%s", pidListValidInterval)
	PidMetricsLog.Infof("proc_pid metrics: procfsRoot=%s", GlobalProcfsRoot)
	PidMetricsLog.Infof("proc_pid metrics: fullMetricsFactor=%d", fullMetricsFactor)
	PidMetricsLog.Infof("proc_pid metrics: activeThreshold=%d", activeThreshold)
	PidMetricsLog.Infof("proc_pid metrics: hostname=%s", GlobalMetricsHostname)
	PidMetricsLog.Infof("proc_pid metrics: job=%s", GlobalMetricsJob)
	PidMetricsLog.Infof("proc_pid metrics: clktckSec=%f", ClktckSec)

	PidMetricsLog.Infof("proc_pid metrics: numGenerators=%d", numGenerators)

	pidMetricsContextList := make([]*PidMetricsContext, numGenerators)
	for pidListPart := 0; pidListPart < numGenerators; pidListPart++ {
		pidMetricsContext, err := NewPidMetricsContext(
			interval,
			GlobalProcfsRoot,
			pidListPart,
			fullMetricsFactor,
			activeThreshold,
			// needed for testing:
			true,
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
		allMetricsGeneratorInfo.Register(PROC_PID_METRICS_GENERATOR_ID_ROOT, 0)
		return nil
	}

	GlobalPidMetricsCtxList = pidMetricsContextList
	for _, pidMetricsCtx := range GlobalPidMetricsCtxList {
		PidMetricsLog.Infof(
			"Start PID metrics generator: interval=%s, partition=%d",
			pidMetricsCtx.interval,
			pidMetricsCtx.pidListPart,
		)
		allMetricsGeneratorInfo.Register(
			pidMetricsCtx.generatorId,
			1,
			fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME, pidMetricsCtx.interval),
			fmt.Sprintf(`%s="%d"`, PROC_PID_METRICS_STATS_PID_LIST_PART_LABEL_NAME, pidMetricsCtx.pidListPart),
			fmt.Sprintf(`%s="%d"`, PROC_PID_METRICS_UP_NUM_GENERATORS_LABEL_NAME, len(pidMetricsContextList)),
			fmt.Sprintf(
				`%s="%s"`,
				INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
				time.Duration(*PidMetricsFullMetricsIntervalArg*float64(time.Second)),
			),
			fmt.Sprintf(`%s="%d"`, INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME, pidMetricsCtx.fullMetricsFactor),
			fmt.Sprintf(`%s="%.02f%%"`, PROC_PID_METRICS_UP_ACTIVE_THRESHOLD_PCT_LABEL_NAME, *PidMetricsActiveThresholdPctArg),
			fmt.Sprintf(`%s="%d"`, PROC_PID_METRICS_UP_ACTIVE_THRESHOLD_LABEL_NAME, pidMetricsCtx.activeThreshold),
		)
		GlobalSchedulerContext.Add(pidMetricsCtx)
	}

	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartPidMetricsFromArgs)
}
