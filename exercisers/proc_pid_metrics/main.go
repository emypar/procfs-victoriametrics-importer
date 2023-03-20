// Exercise proc_pid_metrics in a loop.

package main

import (
	"flag"
	"time"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

var ProcfsRoot = flag.String(
	"procfs-root",
	"/proc",
	"procfs root",
)
var ScanIntervalSeconds = flag.Float64(
	"scan-interval",
	1,
	"How often to scan, in seconds",
)
var FullMetricsFactor = flag.Int(
	"full-metrics-factor",
	15,
	"Generate full metrics every full-metrics-factor pass",
)
var ActiveThreshold = flag.Uint(
	"active-threshold",
	1,
	"Only processes w/ delta CPU time >= threshold are active and have status, io, ... read",
)
var DisplayMetrics = flag.Bool(
	"display-metrics",
	false,
	"Display generated metrics to stdout",
)
var SkipPids = flag.Bool(
	"skip-pids",
	false,
	"Do not process PIDs",
)
var SkipTids = flag.Bool(
	"skip-tids",
	false,
	"Do not process TIDs",
)

func main() {
	flag.Parse()

	scanInterval := time.Duration(*ScanIntervalSeconds * float64(time.Second))
	pidListCacheValidFor := scanInterval / 2
	if pidListCacheValidFor > 1 {
		pidListCacheValidFor = 1
	}

	pvmi.Log.Infof(
		"procfs: %s, scan interval: %.03f sec, full metrics factor: %d, active threshold: %d, skip PIDs/TIDs: %v/%v",
		*ProcfsRoot,
		*ScanIntervalSeconds,
		*FullMetricsFactor,
		*ActiveThreshold,
		*SkipPids,
		*SkipTids,
	)

	pidListNPart, pidListPart := 1, 0
	pidListCacheFlags := uint32(0)
	if !*SkipPids {
		pidListCacheFlags |= pvmi.PID_LIST_CACHE_PID_ENABLED_FLAG
	}
	if !*SkipTids {
		pidListCacheFlags |= pvmi.PID_LIST_CACHE_TID_ENABLED_FLAG
	}
	pidListCache := pvmi.NewPidListCache(
		pidListNPart,
		pidListCacheValidFor,
		*ProcfsRoot,
		pidListCacheFlags,
	)
	wChan := pvmi.DefaultDummyWChan(*DisplayMetrics)
	ctx, err := pvmi.NewPidMetricsContext(
		*ProcfsRoot,
		pidListCache,
		pidListPart,
		*FullMetricsFactor,
		*ActiveThreshold,
		wChan,
	)
	if err != nil {
		pvmi.Log.Fatalf("Failed to create PidMetricsContext: %s", err)
		return
	}

	nextScan := time.Now()
	for {
		pause := time.Until(nextScan)
		if pause > 0 {
			time.Sleep(pause)
		}
		nextScan = nextScan.Add(scanInterval)

		start := time.Now()
		pvmi.GenerateAllPidMetrics(ctx)
		duration := time.Since(start)
		pmStats := ctx.GetStats()
		pvmi.Log.Infof(
			"Duration: %s, Pmc#: %d, PID/TID/Total#: %d/%d/%d , Active#: %d, Full#: %d, Created#: %d, Deleted#: %d, Error#: %d, Metrics#: %d",
			duration,
			pmStats.PmcCount,
			pmStats.PidCount, pmStats.TidCount, (pmStats.PidCount + pmStats.TidCount),
			pmStats.ActiveCount,
			pmStats.FullMetricsCount,
			pmStats.CreatedCount,
			pmStats.DeletedCount,
			pmStats.ErrorCount,
			pmStats.GeneratedCount,
		)
	}
}
