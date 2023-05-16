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
	PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME          = "proc_stat_cpu_time_seconds"
	PROC_STAT_CPU_TIME_PCT_METRIC_NAME              = "proc_stat_cpu_time_pct"
	PROC_STAT_CPU_METRIC_CPU_LABEL_NAME             = "cpu"
	PROC_STAT_CPU_METRIC_CPU_LABEL_TOTAL_VALUE      = "all"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_NAME            = "type"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_GUESTNICE_VALUE = "guest_nice"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_GUEST_VALUE     = "guest"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_IDLE_VALUE      = "idle"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_IOWAIT_VALUE    = "iowait"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_IRQ_VALUE       = "irq"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_NICE_VALUE      = "nice"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_SOFTIRQ_VALUE   = "softirq"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_STEAL_VALUE     = "steal"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_SYSTEM_VALUE    = "system"
	PROC_STAT_CPU_METRIC_TYPE_LABEL_USER_VALUE      = "user"

	PROC_STAT_BOOT_TIME_METRIC_NAME        = "proc_stat_boot_time"
	PROC_STAT_IRQ_TOTAL_METRIC_NAME        = "proc_stat_irq_total_count"
	PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME    = "proc_stat_softirq_total_count"
	PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME = "proc_stat_context_switches_count"
	PROC_STAT_PROCESS_CREATED_METRIC_NAME  = "proc_stat_process_created_count"
	PROC_STAT_PROCESS_RUNNING_METRIC_NAME  = "proc_stat_process_running_count"
	PROC_STAT_PROCESS_BLOCKED_METRIC_NAME  = "proc_stat_process_blocked_count"

	DEFAULT_PROC_STAT_METRICS_SCAN_INTERVAL         = 0.1 // seconds
	DEFAULT_PROC_STAT_METRICS_FULL_METRICS_INTERVAL = 5   // seconds

	CPU_TOTAL_ID = -1

	PROC_STAT_METRICS_GENERATOR_ID = "proc_stat_metrics"
)

var ProcStatMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcStatMetrics",
)

type PCpuStat struct {
	User      float64
	Nice      float64
	System    float64
	Idle      float64
	Iowait    float64
	IRQ       float64
	SoftIRQ   float64
	Steal     float64
	Guest     float64
	GuestNice float64
}

// The context for proc stat metrics generation:
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
	// Use the dual object approach for the parsed data; at the end of the scan
	// the current object is pointed by crtIndex:
	stat     [2]*procfs.Stat2
	crtIndex int
	// Cache previous pCPU:
	prevPCpu map[int]*PCpuStat
	// Stats acquisition time stamp:
	Timestamp time.Time
	// precomputed formats for generating the metrics:
	cpuMetricFmt   string
	pcpuMetricFmt  string
	gaugeMetricFmt string
	// Internal metrics generator ID:
	generatorId string
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
		interval:          interval,
		fullMetricsFactor: fullMetricsFactor,
		fs:                fs,
		prevPCpu:          make(map[int]*PCpuStat),
		cpuMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s",%s="%%s",%s="%%s"} %%.06f %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job, PROC_STAT_CPU_METRIC_CPU_LABEL_NAME, PROC_STAT_CPU_METRIC_TYPE_LABEL_NAME,
		),
		pcpuMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s",%s="%%s",%s="%%s"} %%.02f %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job, PROC_STAT_CPU_METRIC_CPU_LABEL_NAME, PROC_STAT_CPU_METRIC_TYPE_LABEL_NAME,
		),
		gaugeMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s"} %%d %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job,
		),
		generatorId: PROC_STAT_METRICS_GENERATOR_ID,
		wChan:       wChan,
		hostname:    hostname,
		job:         job,
		clktckSec:   clktckSec,
		timeNow:     timeNow,
		bufPool:     bufPool,
	}
	return procStatMetricsCtx, nil
}

func (procStatMetricsCtx *ProcStatMetricsContext) GenerateMetrics() {
	var (
		err        error
		pCpuFactor float64
	)

	metricCount, byteCount := 0, 0
	savedCrtIndex := procStatMetricsCtx.crtIndex
	defer func() {
		if err != nil {
			procStatMetricsCtx.crtIndex = savedCrtIndex
		}

	}()

	prevStat := procStatMetricsCtx.stat[procStatMetricsCtx.crtIndex]
	procStatMetricsCtx.crtIndex = 1 - procStatMetricsCtx.crtIndex
	stat := procStatMetricsCtx.stat[procStatMetricsCtx.crtIndex]
	if stat == nil {
		procStatMetricsCtx.stat[procStatMetricsCtx.crtIndex] = &procfs.Stat2{}
		stat = procStatMetricsCtx.stat[procStatMetricsCtx.crtIndex]
	}

	_, err = procStatMetricsCtx.fs.Stat2(stat)
	if err != nil {
		ProcStatMetricsLog.Warn(err)
		return
	}

	timestamp := procStatMetricsCtx.timeNow().UTC()
	promTs := strconv.FormatInt(timestamp.UnixMilli(), 10)

	bufPool := procStatMetricsCtx.bufPool
	wChan := procStatMetricsCtx.wChan
	buf := bufPool.GetBuffer()

	refreshCycleNum, fullMetricsFactor := procStatMetricsCtx.refreshCycleNum, procStatMetricsCtx.fullMetricsFactor
	statsGroupNum := int64(0)
	gaugeMetricFmt := procStatMetricsCtx.gaugeMetricFmt
	if prevStat == nil || statsGroupNum == refreshCycleNum {
		// Full metrics:
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_BOOT_TIME_METRIC_NAME, stat.BootTime, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_IRQ_TOTAL_METRIC_NAME, stat.IRQTotal, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME, stat.SoftIRQTotal, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME, stat.ContextSwitches, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_CREATED_METRIC_NAME, stat.ProcessCreated, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_RUNNING_METRIC_NAME, stat.ProcessesRunning, promTs)
		fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_BLOCKED_METRIC_NAME, stat.ProcessesBlocked, promTs)
		metricCount += 7
	} else {
		// Deltas:
		if stat.BootTime != prevStat.BootTime {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_BOOT_TIME_METRIC_NAME, stat.BootTime, promTs)
			metricCount += 1
		}
		if stat.IRQTotal != prevStat.IRQTotal {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_IRQ_TOTAL_METRIC_NAME, stat.IRQTotal, promTs)
			metricCount += 1
		}
		if stat.SoftIRQTotal != prevStat.SoftIRQTotal {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_SOFTIRQ_TOTAL_METRIC_NAME, stat.SoftIRQTotal, promTs)
			metricCount += 1
		}
		if stat.ContextSwitches != prevStat.ContextSwitches {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_CONTEXT_SWITCHES_METRIC_NAME, stat.ContextSwitches, promTs)
			metricCount += 1
		}
		if stat.ProcessCreated != prevStat.ProcessCreated {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_CREATED_METRIC_NAME, stat.ProcessCreated, promTs)
			metricCount += 1
		}
		if stat.ProcessesRunning != prevStat.ProcessesRunning {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_RUNNING_METRIC_NAME, stat.ProcessesRunning, promTs)
			metricCount += 1
		}
		if stat.ProcessesBlocked != prevStat.ProcessesBlocked {
			fmt.Fprintf(buf, gaugeMetricFmt, PROC_STAT_PROCESS_BLOCKED_METRIC_NAME, stat.ProcessesBlocked, promTs)
			metricCount += 1
		}
	}
	if statsGroupNum += 1; statsGroupNum >= fullMetricsFactor {
		statsGroupNum = 0
	}

	cpuMetricFmt, pcpuMetricFmt := procStatMetricsCtx.cpuMetricFmt, procStatMetricsCtx.pcpuMetricFmt
	clktckSec := procStatMetricsCtx.clktckSec
	if prevStat != nil {
		// %CPU = (XTime - prevXTime) * clktckSec / (timestamp - prevTimestamp) * 100
		//                             ( <---------------- pCpuFactor -------------> )
		pCpuFactor = clktckSec / timestamp.Sub(procStatMetricsCtx.Timestamp).Seconds() * 100.
	}

	for i, cpuStat := range stat.CPU {
		var (
			cpu               string
			pCpu              float64
			prevCpuStat       *procfs.CPUStat2
			prevPCpu, crtPCpu *PCpuStat
		)

		if cpuStat.Cpu == -1 {
			cpu = "all"
		} else {
			cpu = strconv.Itoa(cpuStat.Cpu)
		}

		if prevStat == nil {
			prevCpuStat = nil
			prevPCpu, crtPCpu = nil, nil
		} else {
			prevCpuStat = prevStat.CPU[i]
			prevPCpu = procStatMetricsCtx.prevPCpu[cpuStat.Cpu]
			if prevPCpu == nil {
				crtPCpu = &PCpuStat{}
				if procStatMetricsCtx.prevPCpu == nil {
					procStatMetricsCtx.prevPCpu = make(map[int]*PCpuStat)
				}
				procStatMetricsCtx.prevPCpu[cpuStat.Cpu] = crtPCpu
			} else {
				crtPCpu = prevPCpu
			}
		}

		if prevStat == nil || statsGroupNum == refreshCycleNum {
			// Full metrics:
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_USER_VALUE, float64(cpuStat.User)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_NICE_VALUE, float64(cpuStat.Nice)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SYSTEM_VALUE, float64(cpuStat.System)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IDLE_VALUE, float64(cpuStat.Idle)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IOWAIT_VALUE, float64(cpuStat.Iowait)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IRQ_VALUE, float64(cpuStat.IRQ)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SOFTIRQ_VALUE, float64(cpuStat.SoftIRQ)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_STEAL_VALUE, float64(cpuStat.Steal)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUEST_VALUE, float64(cpuStat.Guest)*clktckSec, promTs)
			fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUESTNICE_VALUE, float64(cpuStat.GuestNice)*clktckSec, promTs)
			metricCount += 10

			if crtPCpu != nil {
				pCpu = float64(cpuStat.User-prevCpuStat.User) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_USER_VALUE, pCpu, promTs)
				crtPCpu.User = pCpu

				pCpu = float64(cpuStat.Nice-prevCpuStat.Nice) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_NICE_VALUE, pCpu, promTs)
				crtPCpu.Nice = pCpu

				pCpu = float64(cpuStat.System-prevCpuStat.System) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SYSTEM_VALUE, pCpu, promTs)
				crtPCpu.System = pCpu

				pCpu = float64(cpuStat.Idle-prevCpuStat.Idle) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IDLE_VALUE, pCpu, promTs)
				crtPCpu.Idle = pCpu

				pCpu = float64(cpuStat.Iowait-prevCpuStat.Iowait) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IOWAIT_VALUE, pCpu, promTs)
				crtPCpu.Iowait = pCpu

				pCpu = float64(cpuStat.IRQ-prevCpuStat.IRQ) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IRQ_VALUE, pCpu, promTs)
				crtPCpu.IRQ = pCpu

				pCpu = float64(cpuStat.SoftIRQ-prevCpuStat.SoftIRQ) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SOFTIRQ_VALUE, pCpu, promTs)
				crtPCpu.SoftIRQ = pCpu

				pCpu = float64(cpuStat.Steal-prevCpuStat.Steal) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_STEAL_VALUE, pCpu, promTs)
				crtPCpu.Steal = pCpu

				pCpu = float64(cpuStat.Guest-prevCpuStat.Guest) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUEST_VALUE, pCpu, promTs)
				crtPCpu.Guest = pCpu

				pCpu = float64(cpuStat.GuestNice-prevCpuStat.GuestNice) * pCpuFactor
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUESTNICE_VALUE, pCpu, promTs)
				crtPCpu.GuestNice = pCpu

				metricCount += 10
			}
		} else {
			// Delta:
			if cpuStat.User != prevCpuStat.User {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_USER_VALUE, float64(cpuStat.User)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.Nice != prevCpuStat.Nice {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_NICE_VALUE, float64(cpuStat.Nice)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.System != prevCpuStat.System {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SYSTEM_VALUE, float64(cpuStat.System)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.Idle != prevCpuStat.Idle {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IDLE_VALUE, float64(cpuStat.Idle)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.Iowait != prevCpuStat.Iowait {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IOWAIT_VALUE, float64(cpuStat.Iowait)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.IRQ != prevCpuStat.IRQ {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IRQ_VALUE, float64(cpuStat.IRQ)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.SoftIRQ != prevCpuStat.SoftIRQ {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SOFTIRQ_VALUE, float64(cpuStat.SoftIRQ)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.Steal != prevCpuStat.Steal {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_STEAL_VALUE, float64(cpuStat.Steal)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.Guest != prevCpuStat.Guest {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUEST_VALUE, float64(cpuStat.Guest)*clktckSec, promTs)
				metricCount += 1
			}
			if cpuStat.GuestNice != prevCpuStat.GuestNice {
				fmt.Fprintf(buf, cpuMetricFmt, PROC_STAT_CPU_TIME_SECONDS_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUESTNICE_VALUE, float64(cpuStat.GuestNice)*clktckSec, promTs)
				metricCount += 1
			}

			pCpu = float64(cpuStat.User-prevCpuStat.User) * pCpuFactor
			if prevPCpu == nil || int(pCpu*100.) != int(prevPCpu.User*100.) {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_USER_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.User = pCpu

			pCpu = float64(cpuStat.Nice-prevCpuStat.Nice) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.Nice {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_NICE_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.Nice = pCpu

			pCpu = float64(cpuStat.System-prevCpuStat.System) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.System {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SYSTEM_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.System = pCpu

			pCpu = float64(cpuStat.Idle-prevCpuStat.Idle) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.Idle {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IDLE_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.Idle = pCpu

			pCpu = float64(cpuStat.Iowait-prevCpuStat.Iowait) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.Iowait {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IOWAIT_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.Iowait = pCpu

			pCpu = float64(cpuStat.IRQ-prevCpuStat.IRQ) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.IRQ {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_IRQ_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.IRQ = pCpu

			pCpu = float64(cpuStat.SoftIRQ-prevCpuStat.SoftIRQ) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.SoftIRQ {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_SOFTIRQ_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.SoftIRQ = pCpu

			pCpu = float64(cpuStat.Steal-prevCpuStat.Steal) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.Steal {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_STEAL_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.Steal = pCpu

			pCpu = float64(cpuStat.Guest-prevCpuStat.Guest) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.Guest {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUEST_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.Guest = pCpu

			pCpu = float64(cpuStat.GuestNice-prevCpuStat.GuestNice) * pCpuFactor
			if prevPCpu == nil || pCpu != prevPCpu.GuestNice {
				fmt.Fprintf(buf, pcpuMetricFmt, PROC_STAT_CPU_TIME_PCT_METRIC_NAME, cpu, PROC_STAT_CPU_METRIC_TYPE_LABEL_GUESTNICE_VALUE, pCpu, promTs)
				metricCount += 1
			}
			crtPCpu.GuestNice = pCpu
		}

		if statsGroupNum += 1; statsGroupNum >= fullMetricsFactor {
			statsGroupNum = 0
		}
	}

	if refreshCycleNum += 1; refreshCycleNum >= fullMetricsFactor {
		refreshCycleNum = 0
	}
	procStatMetricsCtx.Timestamp = timestamp
	procStatMetricsCtx.refreshCycleNum = refreshCycleNum

	if buf.Len() > 0 {
		buf.WriteByte('\n')
		byteCount += buf.Len()
	}

	if wChan != nil && buf.Len() > 0 {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}
	// Report the internal metrics stats:
	allMetricsGeneratorInfo.Report(procStatMetricsCtx.generatorId, metricCount, byteCount)
}

var ProcStatMetricsScanIntervalArg = flag.Float64(
	"proc-stat-metrics-scan-interval",
	DEFAULT_PROC_STAT_METRICS_SCAN_INTERVAL,
	`proc_stat metrics interval in seconds, use 0 to disable.`,
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
	ProcStatMetricsLog.Infof("proc_stat metrics: interval=%s", interval)
	ProcStatMetricsLog.Infof("proc_stat metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcStatMetricsLog.Infof("proc_stat metrics: hostname=%s", GlobalMetricsHostname)
	ProcStatMetricsLog.Infof("proc_stat metrics: job=%s", GlobalMetricsJob)
	ProcStatMetricsLog.Infof("proc_stat metrics: clktckSec=%f", ClktckSec)
	return procStatMetricsCtx, nil
}

var GlobalProcStatMetricsCtx *ProcStatMetricsContext

func StartProcStatMetricsFromArgs() error {
	procStatMetricsCtx, err := BuildProcStatMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procStatMetricsCtx == nil {
		ProcStatMetricsLog.Warn("proc_stat metrics collection disabled")
		allMetricsGeneratorInfo.Register(
			PROC_STAT_METRICS_GENERATOR_ID,
			0,
		)
		return nil
	}

	allMetricsGeneratorInfo.Register(
		PROC_STAT_METRICS_GENERATOR_ID,
		1,
		fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME, procStatMetricsCtx.interval),
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
			time.Duration(*ProcStatMetricsFullMetricsIntervalArg*float64(time.Second)),
		),
		fmt.Sprintf(
			`%s="%d"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME,
			procStatMetricsCtx.fullMetricsFactor,
		),
	)

	GlobalProcStatMetricsCtx = procStatMetricsCtx
	GlobalSchedulerContext.Add(procStatMetricsCtx)
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcStatMetricsFromArgs)
}
