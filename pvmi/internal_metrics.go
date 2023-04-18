// Internal metrics

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"runtime"
	"strconv"
	"time"
)

const (
	DEFAULT_INTERNAL_METRICS_INTERVAL = 5 // seconds

	INTERNAL_GOROUTINE_COUNT_METRIC_NAME = "pvmi_goroutine_count"

	INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME     = "pvmi_go_mem_stats_total_alloc_bytes"
	INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME             = "pvmi_go_mem_stats_sys_bytes"
	INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME         = "pvmi_go_mem_stats_lookups_total"
	INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME         = "pvmi_go_mem_stats_mallocs_count"
	INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME           = "pvmi_go_mem_stats_frees_count"
	INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME      = "pvmi_go_mem_stats_heap_alloc_count"
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
)

var GoRuntimeIntMetricFmt = fmt.Sprintf(
	`%%s{%s="%%s",%s="%%s"} %%d %%s`+"\n",
	HOSTNAME_LABEL_NAME, JOB_LABEL_NAME,
)

var GoRuntimeFloatMetricFmt = fmt.Sprintf(
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

	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer

	// The following are useful for testing, in lieu of mocks:
	hostname string
	job      string
	bufPool  *BufferPool
}

func (internalMetricsCtx *InternalMetricsContext) GetInterval() time.Duration {
	return internalMetricsCtx.interval
}

var GlobalInternalMetricsCtx *InternalMetricsContext

func BuildInternalMetricsCtxFromArgs() *InternalMetricsContext {
	interval := time.Duration(*InternalMetricsIntervalArg * float64(time.Second))
	if interval <= 0 {
		return nil
	}
	return &InternalMetricsContext{
		interval: interval,
		wChan:    GlobalMetricsWriteChannel,
		hostname: GlobalMetricsHostname,
		job:      GlobalMetricsJob,
		bufPool:  GlobalBufPool,
	}
}

func StartInternalMetricsFromArgs() error {
	internalMetricsCtx := BuildInternalMetricsCtxFromArgs()
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

func GenerateGoRuntimeMetrics(
	internalMetricsCtx *InternalMetricsContext,
	buf *bytes.Buffer,
) {
	hostname, job := internalMetricsCtx.hostname, internalMetricsCtx.job

	numCpus := runtime.NumCPU()
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)

	timestampStr := strconv.FormatInt(time.Now().UnixMilli(), 10)

	intGoRuntimeMetrics := [][2]interface{}{
		{INTERNAL_GOROUTINE_COUNT_METRIC_NAME, numCpus},
		// memStats:
		{INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME, memStats.TotalAlloc},
		{INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME, memStats.Sys},
		{INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME, memStats.Lookups},
		{INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME, memStats.Mallocs},
		{INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME, memStats.Frees},
		{INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME, memStats.HeapAlloc},
		{INTERNAL_GO_MEM_STATS_HEAP_SYS_METRIC_NAME, memStats.HeapSys},
		{INTERNAL_GO_MEM_STATS_HEAP_IDLE_METRIC_NAME, memStats.HeapIdle},
		{INTERNAL_GO_MEM_STATS_HEAP_IN_USE_METRIC_NAME, memStats.HeapInuse},
		{INTERNAL_GO_MEM_STATS_HEAP_RELEASED_METRIC_NAME, memStats.HeapReleased},
		{INTERNAL_GO_MEM_STATS_HEAP_OBJECTS_METRIC_NAME, memStats.HeapObjects},
		{INTERNAL_GO_MEM_STATS_STACK_IN_USE_METRIC_NAME, memStats.StackInuse},
		{INTERNAL_GO_MEM_STATS_STACK_SYS_METRIC_NAME, memStats.StackSys},
		{INTERNAL_GO_MEM_STATS_MSPAN_IN_USE_METRIC_NAME, memStats.MSpanInuse},
		{INTERNAL_GO_MEM_STATS_MSPAN_SYS_METRIC_NAME, memStats.MSpanSys},
		{INTERNAL_GO_MEM_STATS_MCACHE_IN_USE_METRIC_NAME, memStats.MCacheInuse},
		{INTERNAL_GO_MEM_STATS_MCACHE_SYS_METRIC_NAME, memStats.MCacheSys},
		{INTERNAL_GO_MEM_STATS_BUCK_HASH_SYS_METRIC_NAME, memStats.BuckHashSys},
		{INTERNAL_GO_MEM_STATS_GC_SYS_METRIC_NAME, memStats.GCSys},
		{INTERNAL_GO_MEM_STATS_OTHER_SYS_METRIC_NAME, memStats.OtherSys},
		{INTERNAL_GO_MEM_STATS_NEXT_GC_METRIC_NAME, memStats.NextGC},
		{INTERNAL_GO_MEM_STATS_LAST_GC_METRIC_NAME, memStats.LastGC},
		{INTERNAL_GO_MEM_STATS_PAUSE_TOTAL_METRIC_NAME, memStats.PauseTotalNs},
		{INTERNAL_GO_MEM_STATS_NUM_GC_METRIC_NAME, memStats.NumGC},
		{INTERNAL_GO_MEM_STATS_NUM_FORCED_GC_METRIC_NAME, memStats.NumForcedGC},
	}
	floatGoRuntimeMetrics := [][2]interface{}{
		{INTERNAL_GO_MEM_STATS_GC_CPU_FRACTION_METRIC_NAME, memStats.GCCPUFraction},
	}

	for _, memStat := range intGoRuntimeMetrics {
		fmt.Fprintf(buf, GoRuntimeIntMetricFmt, memStat[0], hostname, job, memStat[1], timestampStr)
	}
	for _, memStat := range floatGoRuntimeMetrics {
		fmt.Fprintf(buf, GoRuntimeFloatMetricFmt, memStat[0], hostname, job, memStat[1], timestampStr)
	}

}

func GenerateInternalMetrics(mGenCtx MetricsGenContext) {
	internalMetricsCtx := mGenCtx.(*InternalMetricsContext)
	wChan := internalMetricsCtx.wChan
	bufPool := internalMetricsCtx.bufPool
	buf := bufPool.GetBuffer()

	GenerateGoRuntimeMetrics(internalMetricsCtx, buf)
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
