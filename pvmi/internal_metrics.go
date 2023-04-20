// Internal metrics

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	DEFAULT_INTERNAL_METRICS_INTERVAL = 5 // seconds

	INTERNAL_UP_METRIC_NAME = "pvmi_up"

	INTERNAL_GOROUTINE_COUNT_METRIC_NAME = "pvmi_goroutine_count"

	INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME     = "pvmi_go_mem_stats_total_alloc_bytes"
	INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME             = "pvmi_go_mem_stats_sys_bytes"
	INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME         = "pvmi_go_mem_stats_lookups_total"
	INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME         = "pvmi_go_mem_stats_mallocs_count"
	INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME           = "pvmi_go_mem_stats_frees_count"
	INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME      = "pvmi_go_mem_stats_heap_alloc_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_SYS_METRIC_NAME        = "pvmi_go_mem_stats_heap_sys_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_IDLE_METRIC_NAME       = "pvmi_go_mem_stats_heap_idle_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_IN_USE_METRIC_NAME     = "pvmi_go_mem_stats_heap_in_use_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_RELEASED_METRIC_NAME   = "pvmi_go_mem_stats_heap_released_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_OBJECTS_METRIC_NAME    = "pvmi_go_mem_stats_heap_objects_count"
	INTERNAL_GO_MEM_STATS_STACK_IN_USE_METRIC_NAME    = "pvmi_go_mem_stats_stack_in_use_bytes"
	INTERNAL_GO_MEM_STATS_STACK_SYS_METRIC_NAME       = "pvmi_go_mem_stats_stack_sys_bytes"
	INTERNAL_GO_MEM_STATS_MSPAN_IN_USE_METRIC_NAME    = "pvmi_go_mem_stats_mspan_in_use_bytes"
	INTERNAL_GO_MEM_STATS_MSPAN_SYS_METRIC_NAME       = "pvmi_go_mem_stats_mspan_sys_bytes"
	INTERNAL_GO_MEM_STATS_MCACHE_IN_USE_METRIC_NAME   = "pvmi_go_mem_stats_mcache_in_use_bytes"
	INTERNAL_GO_MEM_STATS_MCACHE_SYS_METRIC_NAME      = "pvmi_go_mem_stats_mcache_sys_bytes"
	INTERNAL_GO_MEM_STATS_BUCK_HASH_SYS_METRIC_NAME   = "pvmi_go_mem_stats_buck_hash_sys_bytes"
	INTERNAL_GO_MEM_STATS_GC_SYS_METRIC_NAME          = "pvmi_go_mem_stats_gc_sys_bytes"
	INTERNAL_GO_MEM_STATS_OTHER_SYS_METRIC_NAME       = "pvmi_go_mem_stats_other_sys_bytes"
	INTERNAL_GO_MEM_STATS_NEXT_GC_METRIC_NAME         = "pvmi_go_mem_stats_next_gc_nanoseconds"
	INTERNAL_GO_MEM_STATS_LAST_GC_METRIC_NAME         = "pvmi_go_mem_stats_last_gc_nanoseconds"
	INTERNAL_GO_MEM_STATS_PAUSE_TOTAL_METRIC_NAME     = "pvmi_go_mem_stats_pause_total_nanoseconds"
	INTERNAL_GO_MEM_STATS_NUM_GC_METRIC_NAME          = "pvmi_go_mem_stats_num_gc_count"
	INTERNAL_GO_MEM_STATS_NUM_FORCED_GC_METRIC_NAME   = "pvmi_go_mem_stats_num_forced_gc_count"
	INTERNAL_GO_MEM_STATS_GC_CPU_FRACTION_METRIC_NAME = "pvmi_go_mem_stats_gc_cpu_fraction"

	INTERNAL_PROC_PCPU_METRIC_NAME  = "pvmi_proc_pcpu"
	INTERNAL_PROC_VSIZE_METRIC_NAME = "pvmi_proc_vsize_bytes"
	INTERNAL_PROC_RSS_METRIC_NAME   = "pvmi_proc_rss_bytes"
)

var InternalMetricIntValFmt = fmt.Sprintf(
	`%%s{%s="%%s",%s="%%s"} %%d %%s`+"\n",
	HOSTNAME_LABEL_NAME, JOB_LABEL_NAME,
)

var InternalMetricFloatValFmt = fmt.Sprintf(
	`%%s{%s="%%s",%s="%%s"} %%f %%s`+"\n",
	HOSTNAME_LABEL_NAME, JOB_LABEL_NAME,
)

var InternalMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"InternalMetrics",
)

var InternalMetricsIntervalArg = flag.Float64(
	"internal-metrics-interval",
	DEFAULT_INTERNAL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often, in seconds, to publish internal metrics.
	`),
)

type InternalMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration

	// The buffer pool:
	bufPool *BufferPool

	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer

	// Internal metrics have a simple format:
	//  <name>{HOSTNAME_LABEL=HOSTNAME,JOB_LABEL=JOB} <val> <ts>
	// so for efficiency purposes pre-build the actual Printf formats for
	// integer and float values:
	intValueMetricFmt   string
	floatValueMetricFmt string

	// For resource utilization like %CPU and memory:
	proc          procfs.Proc
	pid           int
	pageSize      int
	prevStatsTime time.Time
	prevCpuTicks  uint
	clktckSec     float64
	// Process information may not be available when running against a
	// pre-recorded procfs root. If the 1st attempt to read /proc/PID/stat
	// fails, mark this information as unavailable and don't make further
	// attempts.
	procStatUnavailable bool
}

func (internalMetricsCtx *InternalMetricsContext) GetInterval() time.Duration {
	return internalMetricsCtx.interval
}

var GlobalInternalMetricsCtx *InternalMetricsContext

func BuildInternalMetricsCtxFromArgs() (*InternalMetricsContext, error) {
	interval := time.Duration(*InternalMetricsIntervalArg * float64(time.Second))
	if interval <= 0 {
		return nil, nil
	}
	fs, err := procfs.NewFS(GlobalProcfsRoot)
	if err != nil {
		return nil, err
	}

	pid := os.Getpid()
	internalMetricsCtx := &InternalMetricsContext{
		interval: interval,
		bufPool:  GlobalBufPool,
		wChan:    GlobalMetricsWriteChannel,
		intValueMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s"} %%d %%s`+"\n",
			HOSTNAME_LABEL_NAME, GlobalMetricsHostname, JOB_LABEL_NAME, GlobalMetricsJob,
		),
		floatValueMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s"} %%f %%s`+"\n",
			HOSTNAME_LABEL_NAME, GlobalMetricsHostname, JOB_LABEL_NAME, GlobalMetricsJob,
		),
		pid:                 pid,
		pageSize:            os.Getpagesize(),
		clktckSec:           ClktckSec,
		procStatUnavailable: false,
	}

	var procStat procfs.ProcStat
	proc, err := fs.Proc(pid)
	if err == nil {
		procStat, err = proc.Stat()
	}
	if err != nil {
		InternalMetricsLog.Warnf("%%CPU/Mem unavailable: %s", err)
		internalMetricsCtx.procStatUnavailable = true
	} else {
		internalMetricsCtx.prevStatsTime = time.Now()
		internalMetricsCtx.proc = proc
		internalMetricsCtx.prevCpuTicks = procStat.UTime + procStat.STime
	}
	return internalMetricsCtx, nil
}

func StartInternalMetricsFromArgs() error {
	internalMetricsCtx, err := BuildInternalMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if internalMetricsCtx == nil {
		InternalMetricsLog.Warn("Internal metrics disabled")
		return nil
	}
	GlobalInternalMetricsCtx = internalMetricsCtx
	InternalMetricsLog.Infof(
		"Start internal metrics generator: interval=%s",
		GlobalInternalMetricsCtx.interval,
	)
	GlobalSchedulerContext.Add(
		GenerateInternalMetrics,
		MetricsGenContext(GlobalInternalMetricsCtx),
	)
	return nil
}

func GenerateInternalResourceMetrics(
	internalMetricsCtx *InternalMetricsContext,
	buf *bytes.Buffer,
) {
	numCpus := runtime.NumCPU()
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)

	var statsTime time.Time
	var pCpu float64 = -1
	var rss int
	var vSize uint
	if !internalMetricsCtx.procStatUnavailable {
		procStat, err := internalMetricsCtx.proc.Stat()
		statsTime = time.Now()
		if err != nil {
			InternalMetricsLog.Warnf("%%CPU/Mem unavailable: %s", err)
			internalMetricsCtx.procStatUnavailable = true
		} else {
			cpuTicks := procStat.UTime + procStat.STime
			dTime := statsTime.Sub(internalMetricsCtx.prevStatsTime).Seconds()
			pCpu = float64(cpuTicks-internalMetricsCtx.prevCpuTicks) * internalMetricsCtx.clktckSec / dTime * 100.
			internalMetricsCtx.prevStatsTime = statsTime
			internalMetricsCtx.prevCpuTicks = cpuTicks
			rss, vSize = procStat.RSS*internalMetricsCtx.pageSize, procStat.VSize
		}
	} else {
		statsTime = time.Now()
	}

	ts := strconv.FormatInt(statsTime.UnixMilli(), 10)

	intValueMetricFmt := internalMetricsCtx.intValueMetricFmt
	floatValueMetricFmt := internalMetricsCtx.floatValueMetricFmt

	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_UP_METRIC_NAME, 1, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GOROUTINE_COUNT_METRIC_NAME, numCpus, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME, memStats.TotalAlloc, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME, memStats.Sys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME, memStats.Lookups, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME, memStats.Mallocs, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME, memStats.Frees, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME, memStats.HeapAlloc, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_SYS_METRIC_NAME, memStats.HeapSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_IDLE_METRIC_NAME, memStats.HeapIdle, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_IN_USE_METRIC_NAME, memStats.HeapInuse, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_RELEASED_METRIC_NAME, memStats.HeapReleased, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_HEAP_OBJECTS_METRIC_NAME, memStats.HeapObjects, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_STACK_IN_USE_METRIC_NAME, memStats.StackInuse, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_STACK_SYS_METRIC_NAME, memStats.StackSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_MSPAN_IN_USE_METRIC_NAME, memStats.MSpanInuse, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_MSPAN_SYS_METRIC_NAME, memStats.MSpanSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_MCACHE_IN_USE_METRIC_NAME, memStats.MCacheInuse, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_MCACHE_SYS_METRIC_NAME, memStats.MCacheSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_BUCK_HASH_SYS_METRIC_NAME, memStats.BuckHashSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_GC_SYS_METRIC_NAME, memStats.GCSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_OTHER_SYS_METRIC_NAME, memStats.OtherSys, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_NEXT_GC_METRIC_NAME, memStats.NextGC, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_LAST_GC_METRIC_NAME, memStats.LastGC, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_PAUSE_TOTAL_METRIC_NAME, memStats.PauseTotalNs, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_NUM_GC_METRIC_NAME, memStats.NumGC, ts)
	fmt.Fprintf(buf, intValueMetricFmt,
		INTERNAL_GO_MEM_STATS_NUM_FORCED_GC_METRIC_NAME, memStats.NumForcedGC, ts)
	fmt.Fprintf(buf, floatValueMetricFmt,
		INTERNAL_GO_MEM_STATS_GC_CPU_FRACTION_METRIC_NAME, memStats.GCCPUFraction, ts)
	if pCpu >= 0 {
		fmt.Fprintf(buf, floatValueMetricFmt, INTERNAL_PROC_PCPU_METRIC_NAME, pCpu, ts)
		fmt.Fprintf(buf, intValueMetricFmt, INTERNAL_PROC_VSIZE_METRIC_NAME, vSize, ts)
		fmt.Fprintf(buf, intValueMetricFmt, INTERNAL_PROC_RSS_METRIC_NAME, rss, ts)
	}
}

func GenerateInternalMetrics(mGenCtx MetricsGenContext) {
	internalMetricsCtx := mGenCtx.(*InternalMetricsContext)
	wChan := internalMetricsCtx.wChan
	bufPool := internalMetricsCtx.bufPool
	buf := bufPool.GetBuffer()

	GenerateInternalResourceMetrics(internalMetricsCtx, buf)
	// if buf.Len() >= BUF_MAX_SIZE {
	// 	if wChan != nil {
	// 		wChan <- buf
	// 	} else {
	// 		bufPool.ReturnBuffer(buf)
	// 	}
	// 	buf = bufPool.GetBuffer()
	// }

	// Flush the last buffer:
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

}

func init() {
	RegisterStartGeneratorFromArgs(StartInternalMetricsFromArgs)
}
