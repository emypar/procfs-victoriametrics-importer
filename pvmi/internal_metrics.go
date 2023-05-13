// Internal metrics

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/procfs"
)

const (
	DEFAULT_INTERNAL_METRICS_INTERVAL = 5 // seconds

	INTERNAL_METRICS_UP_GROUP_NAME          = "pvmi"
	INTERNAL_METRICS_UP_INTERVAL_LABEL_NAME = "interval"

	INTERNAL_NUM_CPUS_METRIC_NAME                   = "pvmi_num_cpus"
	INTERNAL_NUM_GOROUTINE_METRIC_NAME              = "pvmi_num_goroutine"
	INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME   = "pvmi_go_mem_stats_total_alloc_bytes"
	INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME           = "pvmi_go_mem_stats_sys_bytes"
	INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME       = "pvmi_go_mem_stats_lookups_total"
	INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME       = "pvmi_go_mem_stats_mallocs_count"
	INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME         = "pvmi_go_mem_stats_frees_count"
	INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME    = "pvmi_go_mem_stats_heap_alloc_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_SYS_METRIC_NAME      = "pvmi_go_mem_stats_heap_sys_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_IDLE_METRIC_NAME     = "pvmi_go_mem_stats_heap_idle_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_IN_USE_METRIC_NAME   = "pvmi_go_mem_stats_heap_in_use_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_RELEASED_METRIC_NAME = "pvmi_go_mem_stats_heap_released_bytes"
	INTERNAL_GO_MEM_STATS_HEAP_OBJECTS_METRIC_NAME  = "pvmi_go_mem_stats_heap_objects_count"
	INTERNAL_GO_MEM_STATS_STACK_IN_USE_METRIC_NAME  = "pvmi_go_mem_stats_stack_in_use_bytes"
	INTERNAL_GO_MEM_STATS_STACK_SYS_METRIC_NAME     = "pvmi_go_mem_stats_stack_sys_bytes"
	INTERNAL_GO_MEM_STATS_MSPAN_IN_USE_METRIC_NAME  = "pvmi_go_mem_stats_mspan_in_use_bytes"
	INTERNAL_GO_MEM_STATS_MSPAN_SYS_METRIC_NAME     = "pvmi_go_mem_stats_mspan_sys_bytes"
	INTERNAL_GO_MEM_STATS_MCACHE_IN_USE_METRIC_NAME = "pvmi_go_mem_stats_mcache_in_use_bytes"
	INTERNAL_GO_MEM_STATS_MCACHE_SYS_METRIC_NAME    = "pvmi_go_mem_stats_mcache_sys_bytes"
	INTERNAL_GO_MEM_STATS_BUCK_HASH_SYS_METRIC_NAME = "pvmi_go_mem_stats_buck_hash_sys_bytes"
	INTERNAL_GO_MEM_STATS_GC_SYS_METRIC_NAME        = "pvmi_go_mem_stats_gc_sys_bytes"
	INTERNAL_GO_MEM_STATS_OTHER_SYS_METRIC_NAME     = "pvmi_go_mem_stats_other_sys_bytes"
	INTERNAL_GO_MEM_STATS_NEXT_GC_METRIC_NAME       = "pvmi_go_mem_stats_next_gc_nanoseconds"
	INTERNAL_GO_MEM_STATS_LAST_GC_METRIC_NAME       = "pvmi_go_mem_stats_last_gc_nanoseconds"
	INTERNAL_GO_MEM_STATS_PAUSE_TOTAL_METRIC_NAME   = "pvmi_go_mem_stats_pause_total_nanoseconds"
	INTERNAL_GO_MEM_STATS_NUM_GC_METRIC_NAME        = "pvmi_go_mem_stats_num_gc_count"
	INTERNAL_GO_MEM_STATS_NUM_FORCED_GC_METRIC_NAME = "pvmi_go_mem_stats_num_forced_gc_count"

	INTERNAL_GO_MEM_STATS_NUM_INT_METRICS = 27 // line# 24 .. 50

	INTERNAL_GO_MEM_STATS_GC_CPU_FRACTION_METRIC_NAME = "pvmi_go_mem_stats_gc_cpu_fraction"
	INTERNAL_GO_MEM_STATS_NUM_FLOAT_METRICS           = 1 // line# 53

	INTERNAL_PROC_PCPU_METRIC_NAME  = "pvmi_proc_pcpu"
	INTERNAL_PROC_VSIZE_METRIC_NAME = "pvmi_proc_vsize_bytes"
	INTERNAL_PROC_RSS_METRIC_NAME   = "pvmi_proc_rss_bytes"
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

// <metric_group>_up stats:
type MetricsUp struct {
	metrics map[string]int
	lck     *sync.Mutex
}

func (mu *MetricsUp) Register(metricsGroup string, val int, extraLabels ...string) {
	mu.lck.Lock()
	metric := fmt.Sprintf(
		`%s_up{%s="%s",%s="%s"`,
		metricsGroup, HOSTNAME_LABEL_NAME, GlobalMetricsHostname, JOB_LABEL_NAME, GlobalMetricsJob,
	)
	for _, label := range extraLabels {
		metric += "," + label
	}
	metric += "}"
	mu.metrics[metric] = val
	InternalMetricsLog.Infof("register: %s=%d", metric, val)
	mu.lck.Unlock()
}

var metricsUp = MetricsUp{
	metrics: make(map[string]int),
	lck:     &sync.Mutex{},
}

type InternalMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration

	// The buffer pool:
	bufPool *BufferPool

	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer

	// Internal metrics have a simple format:
	//  <name>{HOSTNAME_LABEL=HOSTNAME,JOB_LABEL=JOB} <val> <promTs>
	// so for efficiency purposes pre-build the actual Printf formats for
	// integer and float values:
	goMetricsFmt  string
	procMetricFmt string

	// For resource utilization like %CPU and memory:
	proc          procfs.Proc
	procStat      procfs.ProcStat
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

	// Build GO metrics format:
	intValueMetricFmt := fmt.Sprintf(
		`%%s{%s="%s",%s="%s"} %%d %%s`+"\n",
		HOSTNAME_LABEL_NAME, GlobalMetricsHostname, JOB_LABEL_NAME, GlobalMetricsJob,
	)
	floatValueMetricFmt := fmt.Sprintf(
		`%%s{%s="%s",%s="%s"} %%f %%s`+"\n",
		HOSTNAME_LABEL_NAME, GlobalMetricsHostname, JOB_LABEL_NAME, GlobalMetricsJob,
	)
	goMetricsFmt := ""
	for i := 0; i < INTERNAL_GO_MEM_STATS_NUM_INT_METRICS; i++ {
		goMetricsFmt += intValueMetricFmt
	}
	for i := 0; i < INTERNAL_GO_MEM_STATS_NUM_FLOAT_METRICS; i++ {
		goMetricsFmt += floatValueMetricFmt
	}

	procMetricsFmt := floatValueMetricFmt + intValueMetricFmt + intValueMetricFmt

	pid := os.Getpid()
	internalMetricsCtx := &InternalMetricsContext{
		interval:            interval,
		bufPool:             GlobalBufPool,
		wChan:               GlobalMetricsWriteChannel,
		goMetricsFmt:        goMetricsFmt,
		procMetricFmt:       procMetricsFmt,
		pid:                 pid,
		pageSize:            os.Getpagesize(),
		clktckSec:           ClktckSec,
		procStatUnavailable: false,
	}

	var procStat procfs.ProcStat
	proc, err := fs.Proc(pid)
	if err == nil {
		_, err = proc.StatWithStruct(&internalMetricsCtx.procStat)
	}
	if err != nil {
		InternalMetricsLog.Warnf("%%CPU/Mem unavailable: %s", err)
		internalMetricsCtx.procStatUnavailable = true
	} else {
		internalMetricsCtx.prevStatsTime = time.Now().UTC()
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
	metricsUp.Register(
		INTERNAL_METRICS_UP_GROUP_NAME,
		1,
		fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_UP_INTERVAL_LABEL_NAME, GlobalInternalMetricsCtx.interval),
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
	numGroutine := runtime.NumGoroutine()
	memStats := runtime.MemStats{}
	runtime.ReadMemStats(&memStats)

	var statsTime time.Time
	var pCpu float64 = -1
	var rss int
	var vSize uint
	if !internalMetricsCtx.procStatUnavailable {
		procStat, err := internalMetricsCtx.proc.StatWithStruct(&internalMetricsCtx.procStat)
		statsTime = time.Now().UTC()
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

	promTs := strconv.FormatInt(statsTime.UnixMilli(), 10)

	fmt.Fprintf(
		buf, internalMetricsCtx.goMetricsFmt,
		INTERNAL_NUM_CPUS_METRIC_NAME, numCpus, promTs,
		INTERNAL_NUM_GOROUTINE_METRIC_NAME, numGroutine, promTs,
		INTERNAL_GO_MEM_STATS_TOTAL_ALLOC_METRIC_NAME, memStats.TotalAlloc, promTs,
		INTERNAL_GO_MEM_STATS_SYS_METRIC_NAME, memStats.Sys, promTs,
		INTERNAL_GO_MEM_STATS_LOOKUPS_METRIC_NAME, memStats.Lookups, promTs,
		INTERNAL_GO_MEM_STATS_MALLOCS_METRIC_NAME, memStats.Mallocs, promTs,
		INTERNAL_GO_MEM_STATS_FREES_METRIC_NAME, memStats.Frees, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_ALLOC_METRIC_NAME, memStats.HeapAlloc, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_SYS_METRIC_NAME, memStats.HeapSys, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_IDLE_METRIC_NAME, memStats.HeapIdle, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_IN_USE_METRIC_NAME, memStats.HeapInuse, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_RELEASED_METRIC_NAME, memStats.HeapReleased, promTs,
		INTERNAL_GO_MEM_STATS_HEAP_OBJECTS_METRIC_NAME, memStats.HeapObjects, promTs,
		INTERNAL_GO_MEM_STATS_STACK_IN_USE_METRIC_NAME, memStats.StackInuse, promTs,
		INTERNAL_GO_MEM_STATS_STACK_SYS_METRIC_NAME, memStats.StackSys, promTs,
		INTERNAL_GO_MEM_STATS_MSPAN_IN_USE_METRIC_NAME, memStats.MSpanInuse, promTs,
		INTERNAL_GO_MEM_STATS_MSPAN_SYS_METRIC_NAME, memStats.MSpanSys, promTs,
		INTERNAL_GO_MEM_STATS_MCACHE_IN_USE_METRIC_NAME, memStats.MCacheInuse, promTs,
		INTERNAL_GO_MEM_STATS_MCACHE_SYS_METRIC_NAME, memStats.MCacheSys, promTs,
		INTERNAL_GO_MEM_STATS_BUCK_HASH_SYS_METRIC_NAME, memStats.BuckHashSys, promTs,
		INTERNAL_GO_MEM_STATS_GC_SYS_METRIC_NAME, memStats.GCSys, promTs,
		INTERNAL_GO_MEM_STATS_OTHER_SYS_METRIC_NAME, memStats.OtherSys, promTs,
		INTERNAL_GO_MEM_STATS_NEXT_GC_METRIC_NAME, memStats.NextGC, promTs,
		INTERNAL_GO_MEM_STATS_LAST_GC_METRIC_NAME, memStats.LastGC, promTs,
		INTERNAL_GO_MEM_STATS_PAUSE_TOTAL_METRIC_NAME, memStats.PauseTotalNs, promTs,
		INTERNAL_GO_MEM_STATS_NUM_GC_METRIC_NAME, memStats.NumGC, promTs,
		INTERNAL_GO_MEM_STATS_NUM_FORCED_GC_METRIC_NAME, memStats.NumForcedGC, promTs,
		INTERNAL_GO_MEM_STATS_GC_CPU_FRACTION_METRIC_NAME, memStats.GCCPUFraction, promTs,
	)
	if pCpu >= 0 {
		fmt.Fprintf(
			buf, internalMetricsCtx.procMetricFmt,
			INTERNAL_PROC_PCPU_METRIC_NAME, pCpu, promTs,
			INTERNAL_PROC_VSIZE_METRIC_NAME, vSize, promTs,
			INTERNAL_PROC_RSS_METRIC_NAME, rss, promTs,
		)
	}
	buf.WriteByte('\n')

	metricsUp.lck.Lock()
	for metric, val := range metricsUp.metrics {
		fmt.Fprintf(buf, "%s %d %s\n", metric, val, promTs)
	}
	metricsUp.lck.Unlock()
	buf.WriteByte('\n')
}

func (internalMetricsCtx *InternalMetricsContext) GenerateMetrics() {
	wChan := internalMetricsCtx.wChan
	bufPool := internalMetricsCtx.bufPool
	buf := bufPool.GetBuffer()

	GenerateInternalResourceMetrics(internalMetricsCtx, buf)

	// Flush the last buffer:
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

}

func GenerateInternalMetrics(mGenCtx MetricsGenContext) {
	mGenCtx.(*InternalMetricsContext).GenerateMetrics()
}

func init() {
	RegisterStartGeneratorFromArgs(StartInternalMetricsFromArgs)
}
