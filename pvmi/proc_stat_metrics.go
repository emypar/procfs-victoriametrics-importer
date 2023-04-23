// /proc/stat metrics

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	PROC_STAT_CPU_USER_TIME_METRIC_NAME           = "proc_stat_cpu_user_time_seconds"
	PROC_STAT_CPU_NICE_TIME_METRIC_NAME           = "proc_stat_cpu_nice_time_seconds"
	PROC_STAT_CPU_SYSTEM_TIME_METRIC_NAME         = "proc_stat_cpu_system_time_seconds"
	PROC_STAT_CPU_IDLE_TIME_METRIC_NAME           = "proc_stat_cpu_idle_time_seconds"
	PROC_STAT_CPU_IOWAIT_TIME_METRIC_NAME         = "proc_stat_cpu_iowait_time_seconds"
	PROC_STAT_CPU_IRQ_TIME_METRIC_NAME            = "proc_stat_cpu_irq_time_seconds"
	PROC_STAT_CPU_SOFTIRQ_TIME_METRIC_NAME        = "proc_stat_cpu_softirq_time_seconds"
	PROC_STAT_CPU_STEAL_TIME_METRIC_NAME          = "proc_stat_cpu_steal_time_seconds"
	PROC_STAT_CPU_GUEST_TIME_METRIC_NAME          = "proc_stat_cpu_guest_time_seconds"
	PROC_STAT_CPU_GUEST_NICE_TIME_METRIC_NAME     = "proc_stat_cpu_guest_nice_time_seconds"
	PROC_STAT_CPU_USER_TIME_PCT_METRIC_NAME       = "proc_stat_cpu_user_time_pct"
	PROC_STAT_CPU_NICE_TIME_PCT_METRIC_NAME       = "proc_stat_cpu_nice_time_pct"
	PROC_STAT_CPU_SYSTEM_TIME_PCT_METRIC_NAME     = "proc_stat_cpu_system_time_pct"
	PROC_STAT_CPU_IDLE_TIME_PCT_METRIC_NAME       = "proc_stat_cpu_idle_time_pct"
	PROC_STAT_CPU_IOWAIT_TIME_PCT_METRIC_NAME     = "proc_stat_cpu_iowait_time_pct"
	PROC_STAT_CPU_IRQ_TIME_PCT_METRIC_NAME        = "proc_stat_cpu_irq_time_pct"
	PROC_STAT_CPU_SOFTIRQ_TIME_PCT_METRIC_NAME    = "proc_stat_cpu_softirq_time_pct"
	PROC_STAT_CPU_STEAL_TIME_PCT_METRIC_NAME      = "proc_stat_cpu_steal_time_pct"
	PROC_STAT_CPU_GUEST_TIME_PCT_METRIC_NAME      = "proc_stat_cpu_guest_time_pct"
	PROC_STAT_CPU_GUEST_NICE_TIME_PCT_METRIC_NAME = "proc_stat_cpu_guest_nice_time_pct"
	PROC_STAT_CPU_METRIC_CPU_LABEL_NAME           = "cpu"
	PROC_STAT_CPU_METRIC_CPU_LABEL_TOTAL_VALUE    = "all"

	PROC_STAT_BOOT_TIME_METRIC_NAME        = "proc_stat_boot_time"
	PROC_STAT_IRQ_TOTAL_METRIC_NAME        = "proc_stat_irq_total_count"
	PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME    = "proc_stat_softirq_total_count"
	PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME = "proc_stat_context_switches_count"
	PROC_STAT_PROCESS_CREATED_METRIC_NAME  = "proc_stat_process_created_count"
	PROC_STAT_PROCESS_RUNNING_METRIC_NAME  = "proc_stat_process_running_count"
	PROC_STAT_PROCESS_BLOCKED_METRIC_NAME  = "proc_stat_process_blocked_count"

	DEFAULT_PROC_STAT_METRICS_SCAN_INTERVAL         = 0.1 // seconds
	DEFAULT_PROC_STAT_METRICS_FULL_METRICS_INTERVAL = 5   // seconds

)

var ProcStatMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcStatMetrics",
)

// The context for pid metrics generation:
type ProcStatMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration
	// Full metrics factor, N. For delta strategy, metrics are generated only
	// for stats that changed from the previous run, however the full set is
	// generated at every Nth pass. Use  N = 1 for full metrics every time.
	fullMetricsFactor int64
	// The refresh cycle#, modulo fullMetricsFactor. Metrics are bundled
	// together in groups with group# also modulo fullMetricsFactor. Each time
	// group# == refresh cycle, all the metrics in the group are generated
	// regardless whether they changed or not from the previous scan:
	refreshCycleNum int64
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// Cache previous parsed data for delta and/or %:
	prevStat *procfs.Stat
	// Timestamp of the prev scan:
	prevTs time.Time
	// procfs.Stat returns CPU times computed w/ a predefined constant userHZ
	// which may need a correction factor if the runtime SC_CLK_TCK is
	// different:
	procfsUserHZCorrectionFactor float64
	// precomputed formats for generating the metrics:
	cpuMetricFmt   string
	pcpuMetricFmt  string
	gaugeMetricFmt string
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// The following are useful for testing, in lieu of mocks:
	hostname  string
	job       string
	clktckSec float64
	timeNow   TimeNowFn
	bufPool   *BufferPool
}

func (procStatMetricsCtx *ProcStatMetricsContext) GetInterval() time.Duration {
	return procStatMetricsCtx.interval
}

func NewProcStatMetricsContext(
	interval time.Duration,
	fullMetricsFactor int64,
	procfsRoot string,
	// needed for testing:
	hostname string,
	job string,
	clktckSec float64,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	bufPool *BufferPool,
) (*ProcStatMetricsContext, error) {
	if hostname == "" {
		hostname = GlobalMetricsHostname
	}
	if job == "" {
		job = GlobalMetricsJob
	}
	if clktckSec <= 0 {
		clktckSec = ClktckSec
	}
	if timeNow == nil {
		timeNow = time.Now
	}
	if wChan == nil {
		wChan = GlobalMetricsWriteChannel
	}
	if bufPool == nil {
		bufPool = GlobalBufPool
	}
	fs, err := procfs.NewFS(procfsRoot)
	if err != nil {
		return nil, err
	}
	procStatMetricsCtx := &ProcStatMetricsContext{
		interval:                     interval,
		fullMetricsFactor:            fullMetricsFactor,
		fs:                           fs,
		procfsUserHZCorrectionFactor: float64(PROCFS_USER_HZ) * clktckSec,
		cpuMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s",%s="%%s"} %%.06f %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job, PROC_STAT_CPU_METRIC_CPU_LABEL_NAME,
		),
		pcpuMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s",%s="%%s"} %%.02f %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job, PROC_STAT_CPU_METRIC_CPU_LABEL_NAME,
		),
		gaugeMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s"} %%d %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job,
		),
		wChan:     wChan,
		hostname:  hostname,
		job:       job,
		clktckSec: clktckSec,
		timeNow:   timeNow,
		bufPool:   bufPool,
	}
	return procStatMetricsCtx, nil
}

func GenerateProcStatMetrics(mGenCtx MetricsGenContext) {
	procStatMetricsCtx := mGenCtx.(*ProcStatMetricsContext)
	stat, err := procStatMetricsCtx.fs.Stat()
	if err != nil {
		ProcStatMetricsLog.Warn(err)
		return
	}
	statsTs := procStatMetricsCtx.timeNow()
	promTs := strconv.FormatInt(statsTs.UnixMilli(), 10)

	bufPool := procStatMetricsCtx.bufPool
	wChan := procStatMetricsCtx.wChan
	buf := bufPool.GetBuffer()

	prevStat := procStatMetricsCtx.prevStat
	pctFactor := float64(0)
	if prevStat != nil {
		pctFactor = 100. / statsTs.Sub(procStatMetricsCtx.prevTs).Seconds()
	}

	refreshCycleNum, fullMetricsFactor := procStatMetricsCtx.refreshCycleNum, procStatMetricsCtx.fullMetricsFactor
	statsGroupNum := int64(0)

	gaugeMetricFmt := procStatMetricsCtx.gaugeMetricFmt
	if prevStat == nil {
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_BOOT_TIME_METRIC_NAME, stat.BootTime, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_IRQ_TOTAL_METRIC_NAME, stat.IRQTotal, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME, stat.SoftIRQTotal, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME, stat.ContextSwitches, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_CREATED_METRIC_NAME, stat.ProcessCreated, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_RUNNING_METRIC_NAME, stat.ProcessesRunning, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_BLOCKED_METRIC_NAME, stat.ProcessesBlocked, promTs)
	} else {
		fullMetrics := statsGroupNum == refreshCycleNum
		if fullMetrics || stat.BootTime != prevStat.BootTime {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_BOOT_TIME_METRIC_NAME, stat.BootTime, promTs)
		}
		if fullMetrics || stat.IRQTotal != prevStat.IRQTotal {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_IRQ_TOTAL_METRIC_NAME, stat.IRQTotal, promTs)
		}
		if fullMetrics || stat.SoftIRQTotal != prevStat.SoftIRQTotal {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME, stat.SoftIRQTotal, promTs)
		}
		if fullMetrics || stat.ContextSwitches != prevStat.ContextSwitches {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME, stat.ContextSwitches, promTs)
		}
		if fullMetrics || stat.ProcessCreated != prevStat.ProcessCreated {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_CREATED_METRIC_NAME, stat.ProcessCreated, promTs)
		}
		if fullMetrics || stat.ProcessesRunning != prevStat.ProcessesRunning {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_RUNNING_METRIC_NAME, stat.ProcessesRunning, promTs)
		}
		if fullMetrics || stat.ProcessesBlocked != prevStat.ProcessesBlocked {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_BLOCKED_METRIC_NAME, stat.ProcessesBlocked, promTs)
		}
	}
	if statsGroupNum += 1; statsGroupNum >= fullMetricsFactor {
		statsGroupNum = 0
	}

	cpuMetricsWithCorrection := func(cpuStat *procfs.CPUStat, prevCpuStat *procfs.CPUStat, cpu string) {
		fullMetrics := statsGroupNum == refreshCycleNum

		procfsUserHZCorrectionFactor := procStatMetricsCtx.procfsUserHZCorrectionFactor
		if procfsUserHZCorrectionFactor != 1 {
			cpuStat.User *= procfsUserHZCorrectionFactor
			cpuStat.Nice *= procfsUserHZCorrectionFactor
			cpuStat.System *= procfsUserHZCorrectionFactor
			cpuStat.Idle *= procfsUserHZCorrectionFactor
			cpuStat.Iowait *= procfsUserHZCorrectionFactor
			cpuStat.IRQ *= procfsUserHZCorrectionFactor
			cpuStat.SoftIRQ *= procfsUserHZCorrectionFactor
			cpuStat.Steal *= procfsUserHZCorrectionFactor
			cpuStat.Guest *= procfsUserHZCorrectionFactor
			cpuStat.GuestNice *= procfsUserHZCorrectionFactor
		}

		cpuMetricFmt := procStatMetricsCtx.cpuMetricFmt
		pcpuMetricFmt := procStatMetricsCtx.pcpuMetricFmt

		if prevCpuStat == nil {
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_USER_TIME_METRIC_NAME, cpu, cpuStat.User, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_NICE_TIME_METRIC_NAME, cpu, cpuStat.Nice, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_SYSTEM_TIME_METRIC_NAME, cpu, cpuStat.System, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IDLE_TIME_METRIC_NAME, cpu, cpuStat.Idle, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IOWAIT_TIME_METRIC_NAME, cpu, cpuStat.Iowait, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IRQ_TIME_METRIC_NAME, cpu, cpuStat.IRQ, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_SOFTIRQ_TIME_METRIC_NAME, cpu, cpuStat.SoftIRQ, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_STEAL_TIME_METRIC_NAME, cpu, cpuStat.Steal, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_GUEST_TIME_METRIC_NAME, cpu, cpuStat.Guest, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_GUEST_NICE_TIME_METRIC_NAME, cpu, cpuStat.GuestNice, promTs)
		} else {
			if fullMetrics || cpuStat.User != prevCpuStat.User {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_USER_TIME_METRIC_NAME, cpu, cpuStat.User, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_USER_TIME_PCT_METRIC_NAME, cpu, (cpuStat.User-prevCpuStat.User)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.Nice != prevCpuStat.Nice {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_NICE_TIME_METRIC_NAME, cpu, cpuStat.Nice, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_NICE_TIME_PCT_METRIC_NAME, cpu, (cpuStat.Nice-prevCpuStat.Nice)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.System != prevCpuStat.System {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_SYSTEM_TIME_METRIC_NAME, cpu, cpuStat.System, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_SYSTEM_TIME_PCT_METRIC_NAME, cpu, (cpuStat.System-prevCpuStat.System)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.Idle != prevCpuStat.Idle {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IDLE_TIME_METRIC_NAME, cpu, cpuStat.Idle, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_IDLE_TIME_PCT_METRIC_NAME, cpu, (cpuStat.Idle-prevCpuStat.Idle)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.Iowait != prevCpuStat.Iowait {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IOWAIT_TIME_METRIC_NAME, cpu, cpuStat.Iowait, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_IOWAIT_TIME_PCT_METRIC_NAME, cpu, (cpuStat.Iowait-prevCpuStat.Iowait)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.IRQ != prevCpuStat.IRQ {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_IRQ_TIME_METRIC_NAME, cpu, cpuStat.IRQ, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_IRQ_TIME_PCT_METRIC_NAME, cpu, (cpuStat.IRQ-prevCpuStat.IRQ)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.SoftIRQ != prevCpuStat.SoftIRQ {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_SOFTIRQ_TIME_METRIC_NAME, cpu, cpuStat.SoftIRQ, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_SOFTIRQ_TIME_PCT_METRIC_NAME, cpu, (cpuStat.SoftIRQ-prevCpuStat.SoftIRQ)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.Steal != prevCpuStat.Steal {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_STEAL_TIME_METRIC_NAME, cpu, cpuStat.Steal, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_STEAL_TIME_PCT_METRIC_NAME, cpu, (cpuStat.Steal-prevCpuStat.Steal)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.Guest != prevCpuStat.Guest {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_GUEST_TIME_METRIC_NAME, cpu, cpuStat.Guest, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_GUEST_TIME_PCT_METRIC_NAME, cpu, (cpuStat.Guest-prevCpuStat.Guest)*pctFactor, promTs)
			}
			if fullMetrics || cpuStat.GuestNice != prevCpuStat.GuestNice {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_GUEST_TIME_METRIC_NAME, cpu, cpuStat.GuestNice, promTs)
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_GUEST_TIME_PCT_METRIC_NAME, cpu, (cpuStat.GuestNice-prevCpuStat.GuestNice)*pctFactor, promTs)
			}
		}
	}

	var prevCpuStat *procfs.CPUStat = nil

	if prevStat != nil {
		prevCpuStat = &prevStat.CPUTotal
	}
	cpuMetricsWithCorrection(&stat.CPUTotal, prevCpuStat, PROC_STAT_CPU_METRIC_CPU_LABEL_TOTAL_VALUE)

	// For testing purposes statsGroupNum should be predictable; however the CPU
	// stats are iterated in hash order, so rather then use the order for
	// statsGroupNum, we will use the statsGroupNum + CPU#. This way the test
	// program can determine which CPU should be fully refreshed for a given cycle.
	cpuStatGroupNumBase := statsGroupNum + 1
	for i, cpuStat := range stat.CPU {
		if prevStat != nil {
			*prevCpuStat = prevStat.CPU[i]
		}
		if statsGroupNum = cpuStatGroupNumBase + i; statsGroupNum >= fullMetricsFactor {
			statsGroupNum = 0
		}
		cpuMetricsWithCorrection(&cpuStat, prevCpuStat, strconv.FormatInt(i, 10))
	}

	procStatMetricsCtx.prevStat = &stat
	procStatMetricsCtx.prevTs = statsTs
	if refreshCycleNum += 1; refreshCycleNum >= fullMetricsFactor {
		refreshCycleNum = 0
	}
	procStatMetricsCtx.refreshCycleNum = refreshCycleNum

	if wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}
}

var ProcStatMetricsScanIntervalArg = flag.Float64(
	"proc-stat-metrics-scan-interval",
	DEFAULT_PROC_STAT_METRICS_SCAN_INTERVAL,
	`Proc stat metrics interval in seconds, use 0 to disable.`,
)

var ProcStatMetricsFullMetricsIntervalArg = flag.Float64(
	"proc-stat-metrics-full-metrics-interval",
	DEFAULT_PROC_STAT_METRICS_FULL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often to generate full metrics, in seconds; normally only the metrics
	whose value has changed from the previous scan are generated, but every
	so often the entire set is generated to prevent queries from having to go
	too much back in time to find the last value. Use 0 to generate full
	metrics at every scan.
	`),
)

func BuildProcStatMetricsCtxFromArgs() (*ProcStatMetricsContext, error) {
	if *ProcStatMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*ProcStatMetricsScanIntervalArg * float64(time.Second))
	fullMetricsInterval := time.Duration(*ProcStatMetricsFullMetricsIntervalArg * float64(time.Second))
	fullMetricsFactor := int64(fullMetricsInterval / interval)
	if fullMetricsFactor < 1 {
		fullMetricsFactor = 1
	}

	procStatMetricsCtx, err := NewProcStatMetricsContext(
		interval,
		fullMetricsFactor,
		GlobalProcfsRoot,
		// needed for testing:
		GlobalMetricsHostname,
		GlobalMetricsJob,
		ClktckSec,
		// will be set to default values:
		nil, // timeNow TimeNowFn,
		nil, // wChan chan *bytes.Buffer,
		nil, // bufPool *BufferPool,
	)
	if err != nil {
		return nil, err
	}
	ProcStatMetricsLog.Infof("proc_stat_metrics: interval=%s", interval)
	ProcStatMetricsLog.Infof("proc_stat_metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcStatMetricsLog.Infof("proc_stat_metrics: hostname=%s", GlobalMetricsHostname)
	ProcStatMetricsLog.Infof("proc_stat_metrics: job=%s", GlobalMetricsJob)
	ProcStatMetricsLog.Infof("proc_stat_metrics: clktckSec=%f", ClktckSec)
	return procStatMetricsCtx, nil
}

var GlobalProcStatMetricsCtx *ProcStatMetricsContext

func StartProcStatMetricsFromArgs() error {
	procStatMetricsCtx, err := BuildProcStatMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procStatMetricsCtx == nil {
		ProcStatMetricsLog.Warn("Proc stat metrics collection disabled")
		return nil
	}

	GlobalProcStatMetricsCtx = procStatMetricsCtx
	GlobalSchedulerContext.Add(GenerateProcStatMetrics, MetricsGenContext(procStatMetricsCtx))
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcStatMetricsFromArgs)
}
