// Internal metrics

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/procfs"
)

const (
	DEFAULT_INTERNAL_METRICS_INTERVAL = 5 // seconds

	INTERNAL_METRICS_GENERATOR_ID = "internal_metrics"

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

	// Each metrics generator has set a of standard internal metrics that have
	// an ID (set of) label name="value"(s):
	INTERNAL_METRICS_GENERATOR_UP_METRIC_NAME                          = "pvmi_up"
	INTERNAL_METRICS_GENERATOR_METRIC_COUNT_METRIC_NAME                = "pvmi_metric_count"
	INTERNAL_METRICS_GENERATOR_BYTE_COUNT_METRIC_NAME                  = "pvmi_byte_count"
	INTERNAL_METRICS_GENERATOR_SKIPPED_COUNT_METRIC_NAME               = "pvmi_skipped_count"
	INTERNAL_METRICS_GENERATOR_ID_LABEL_NAME                           = "generator"
	INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME              = "interval"
	INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME = "full_metrics_interval"
	INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME   = "full_metrics_factor"
)

type MetricsGeneratorInfo struct {
	// Up value: 0/1:
	up int
	// Common labels: hostname="...",job="...",generator="..."
	commonLabels string
	// Configuration labels: [,param="..."*]. Note the starting "," such that
	// they can be appended to common labels:
	configLabels string
	// The number of generated metrics:
	metricCount int
	// The number of bytes for all metrics:
	byteCount uint64
	// The number of skipped generation cycles, due to scheduler overlap (a new
	// scan would have been started before the previous one was completed):
	skippedCount int
}

type AllMetricsGeneratorInfo struct {
	// Info map:
	info map[string]*MetricsGeneratorInfo
	// Lock protection:
	lck *sync.Mutex
}

func (allMgi *AllMetricsGeneratorInfo) Register(generatorId string, up int, configLabels ...string) {
	allMgi.lck.Lock()
	if allMgi.info[generatorId] == nil {
		allMgi.info[generatorId] = &MetricsGeneratorInfo{}
	}
	mgi := allMgi.info[generatorId]
	mgi.up = up
	mgi.commonLabels = fmt.Sprintf(
		`%s="%s",%s="%s",%s="%s"`,
		HOSTNAME_LABEL_NAME, GlobalMetricsHostname,
		JOB_LABEL_NAME, GlobalMetricsJob,
		INTERNAL_METRICS_GENERATOR_ID_LABEL_NAME, generatorId,
	)
	if len(configLabels) > 0 {
		mgi.configLabels = "," + strings.Join(configLabels, ",")
	}
	mgi.metricCount = 0
	mgi.byteCount = 0
	allMgi.lck.Unlock()
}

func (allMgi *AllMetricsGeneratorInfo) Report(generatorId string, metricCount int, byteCount int) {
	allMgi.lck.Lock()
	mgi := allMgi.info[generatorId]
	if mgi != nil {
		mgi.metricCount += metricCount
		mgi.byteCount += uint64(byteCount)

	}
	allMgi.lck.Unlock()
}

func (allMgi *AllMetricsGeneratorInfo) ReportSkip(generatorId string) {
	allMgi.lck.Lock()
	mgi := allMgi.info[generatorId]
	if mgi != nil {
		mgi.skippedCount += 1
	}
	allMgi.lck.Unlock()
}

func (allMgi *AllMetricsGeneratorInfo) GenerateMetrics(buf *bytes.Buffer) int {
	allMgi.lck.Lock()
	defer allMgi.lck.Unlock()
	promTs := strconv.FormatInt(time.Now().UnixMilli(), 10)
	metricCount := 0
	for _, mgi := range allMgi.info {
		fmt.Fprintf(
			buf,
			"%s{%s%s} %d %s\n%s{%s} %d %s\n%s{%s} %d %s\n%s{%s} %d %s\n",
			INTERNAL_METRICS_GENERATOR_UP_METRIC_NAME, mgi.commonLabels, mgi.configLabels, mgi.up, promTs,
			INTERNAL_METRICS_GENERATOR_METRIC_COUNT_METRIC_NAME, mgi.commonLabels, mgi.metricCount, promTs,
			INTERNAL_METRICS_GENERATOR_BYTE_COUNT_METRIC_NAME, mgi.commonLabels, mgi.byteCount, promTs,
			INTERNAL_METRICS_GENERATOR_SKIPPED_COUNT_METRIC_NAME, mgi.commonLabels, mgi.skippedCount, promTs,
		)
		metricCount += 4
	}
	if metricCount > 0 {
		buf.WriteByte('\n')
	}
	return metricCount
}

var allMetricsGeneratorInfo = &AllMetricsGeneratorInfo{
	info: make(map[string]*MetricsGeneratorInfo),
	lck:  &sync.Mutex{},
}

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

	// Internal metrics generator ID:
	generatorId string
}

func (internalMetricsCtx *InternalMetricsContext) GetInterval() time.Duration {
	return internalMetricsCtx.interval
}

func (internalMetricsCtx *InternalMetricsContext) GetGeneratorId() string {
	return internalMetricsCtx.generatorId
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

	internalMetricsCtx.generatorId = INTERNAL_METRICS_GENERATOR_ID
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
	allMetricsGeneratorInfo.Register(
		GlobalInternalMetricsCtx.generatorId,
		1,
		fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME, GlobalInternalMetricsCtx.interval),
	)
	GlobalSchedulerContext.Add(GlobalInternalMetricsCtx)
	return nil
}

func GenerateInternalResourceMetrics(
	internalMetricsCtx *InternalMetricsContext,
	buf *bytes.Buffer,
) int {
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
		statsTime = time.Now().UTC()
	}

	promTs := strconv.FormatInt(statsTime.UnixMilli(), 10)

	metricCount := 0
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
	metricCount += INTERNAL_GO_MEM_STATS_NUM_INT_METRICS + INTERNAL_GO_MEM_STATS_NUM_FLOAT_METRICS
	if pCpu >= 0 {
		fmt.Fprintf(
			buf, internalMetricsCtx.procMetricFmt,
			INTERNAL_PROC_PCPU_METRIC_NAME, pCpu, promTs,
			INTERNAL_PROC_VSIZE_METRIC_NAME, vSize, promTs,
			INTERNAL_PROC_RSS_METRIC_NAME, rss, promTs,
		)
		metricCount += 3
	}
	buf.WriteByte('\n')
	return metricCount
}

func (internalMetricsCtx *InternalMetricsContext) GenerateMetrics() {
	wChan := internalMetricsCtx.wChan
	bufPool := internalMetricsCtx.bufPool
	buf := bufPool.GetBuffer()

	metricCount := GenerateInternalResourceMetrics(internalMetricsCtx, buf)
	metricCount += allMetricsGeneratorInfo.GenerateMetrics(buf)
	byteCount := buf.Len()

	// Flush the last buffer:
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

	// Report the internal metrics stats:
	allMetricsGeneratorInfo.Report(internalMetricsCtx.generatorId, metricCount, byteCount)
}

func init() {
	RegisterStartGeneratorFromArgs(StartInternalMetricsFromArgs)
}
