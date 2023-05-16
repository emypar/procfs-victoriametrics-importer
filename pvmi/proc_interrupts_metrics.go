// /proc/interrupts

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	PROC_INTERRUPTS_TOTAL_METRIC_NAME       = "proc_interrupts_total"
	PROC_INTERRUPTS_INFO_METRIC_NAME        = "proc_interrupts_info"
	PROC_INTERRUPTS_IRQ_LABEL_NAME          = "interrupt"
	PROC_INTERRUPTS_TOTAL_CPU_LABEL_NAME    = "cpu"
	PROC_INTERRUPTS_INFO_DEVICES_LABEL_NAME = "devices"
	PROC_INTERRUPTS_INFO_INFO_LABEL_NAME    = "info"

	DEFAULT_PROC_INTERRUPTS_METRICS_SCAN_INTERVAL         = 1  // seconds
	DEFAULT_PROC_INTERRUPTS_METRICS_FULL_METRICS_INTERVAL = 15 // seconds

	// Stats metrics:
	PROC_INTERRUPTS_METRICS_GENERATOR_ID = "proc_interrupts_metrics"
)

var ProcInterruptsMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcInterruptsMetrics",
)

// The context for proc net dev metrics generation:
type ProcInterruptsMetricsContext struct {
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
	// The group# above is assigned at the time when an irq is discovered, per
	// irq, based on modulo fullMetricsFactor counter:
	newIrqs             []string
	refreshGroupNum     map[string]int64
	nextRefreshGroupNum int64
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// The previous state, needed for delta strategy:
	prevInterrupts procfs.Interrupts
	// Precomputed formats for generating the metrics:
	counterMetricFmt string
	infoMetricFmt    string
	// Internal metrics generator ID:
	generatorId string
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// The following are useful for testing, in lieu of mocks:
	hostname string
	job      string
	timeNow  TimeNowFn
	bufPool  *BufferPool
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GetInterval() time.Duration {
	return procInterruptsMetricsCtx.interval
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GetGeneratorId() string {
	return procInterruptsMetricsCtx.generatorId
}

func NewProcInterruptsMetricsContext(
	interval time.Duration,
	fullMetricsFactor int64,
	procfsRoot string,
	// needed for testing:
	hostname string,
	job string,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	bufPool *BufferPool,
) (*ProcInterruptsMetricsContext, error) {
	if hostname == "" {
		hostname = GlobalMetricsHostname
	}
	if job == "" {
		job = GlobalMetricsJob
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
	procInterruptsMetricsCtx := &ProcInterruptsMetricsContext{
		interval:            interval,
		fullMetricsFactor:   fullMetricsFactor,
		refreshCycleNum:     0,
		newIrqs:             make([]string, 128)[:0],
		refreshGroupNum:     make(map[string]int64),
		nextRefreshGroupNum: 0,
		fs:                  fs,
		counterMetricFmt: fmt.Sprintf(
			`%s{%s="%s",%s="%s",%s="%%s",%s="%%d"} %%s %%s`+"\n",
			PROC_INTERRUPTS_TOTAL_METRIC_NAME,
			HOSTNAME_LABEL_NAME, hostname,
			JOB_LABEL_NAME, job,
			PROC_INTERRUPTS_IRQ_LABEL_NAME,
			PROC_INTERRUPTS_TOTAL_CPU_LABEL_NAME,
		),
		infoMetricFmt: fmt.Sprintf(
			`%s{%s="%s",%s="%s",%s="%%s",%s="%%s",%s="%%s"} %%d %%s`+"\n",
			PROC_INTERRUPTS_INFO_METRIC_NAME,
			HOSTNAME_LABEL_NAME, hostname,
			JOB_LABEL_NAME, job,
			PROC_INTERRUPTS_IRQ_LABEL_NAME,
			PROC_INTERRUPTS_INFO_DEVICES_LABEL_NAME,
			PROC_INTERRUPTS_INFO_INFO_LABEL_NAME,
		),
		generatorId: PROC_INTERRUPTS_METRICS_GENERATOR_ID,
		wChan:       wChan,
		hostname:    hostname,
		job:         job,
		timeNow:     timeNow,
		bufPool:     bufPool,
	}
	return procInterruptsMetricsCtx, nil
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GenerateMetrics() {
	metricCount, byteCount := 0, 0
	interrupts, err := procInterruptsMetricsCtx.fs.Interrupts()
	if err != nil {
		ProcInterruptsMetricsLog.Warn(err)
		return
	}
	statsTs := procInterruptsMetricsCtx.timeNow()
	promTs := strconv.FormatInt(statsTs.UnixMilli(), 10)

	fullMetricsFactor := procInterruptsMetricsCtx.fullMetricsFactor
	deltaStrategy := fullMetricsFactor > 1

	prevInterrupts := procInterruptsMetricsCtx.prevInterrupts
	newIrqs := procInterruptsMetricsCtx.newIrqs[:0]
	refreshGroupNum := procInterruptsMetricsCtx.refreshGroupNum
	refreshCycleNum := procInterruptsMetricsCtx.refreshCycleNum

	counterMetricFmt := procInterruptsMetricsCtx.counterMetricFmt
	infoMetricFmt := procInterruptsMetricsCtx.infoMetricFmt
	bufPool := procInterruptsMetricsCtx.bufPool
	buf := bufPool.GetBuffer()
	wChan := procInterruptsMetricsCtx.wChan

	for irq, interrupt := range interrupts {
		fullMetrics := !deltaStrategy
		prevInterrupt, exists := prevInterrupts[irq]
		if deltaStrategy {
			if exists {
				fullMetrics = refreshGroupNum[irq] == refreshCycleNum
			} else {
				fullMetrics = true
				newIrqs = append(newIrqs, irq)
			}
		}
		// Handle pseudo-categorical info 1st:
		if exists && (prevInterrupt.Devices != interrupt.Devices || prevInterrupt.Info != interrupt.Info) {
			fmt.Fprintf(buf, infoMetricFmt, irq, prevInterrupt.Devices, prevInterrupt.Info, 0, promTs)
			metricCount += 1
		}
		if fullMetrics {
			fmt.Fprintf(buf, infoMetricFmt, irq, interrupt.Devices, interrupt.Info, 1, promTs)
			metricCount += 1
		}
		// Values:
		for cpu, value := range interrupt.Values {
			if fullMetrics || prevInterrupt.Values[cpu] != value {
				fmt.Fprintf(buf, counterMetricFmt, irq, cpu, value, promTs)
				metricCount += 1
			}
		}
		// Remove the IRQ from previous stats, such that at the end what's left
		// has to be cleared:
		if exists {
			delete(prevInterrupts, irq)
		}
	}
	// Handle out-of scope interrupts left in prevInterrupts. This may be an
	// overkill since interrupt sources do not vanish on the fly.
	if len(prevInterrupts) > 0 {
		for irq, prevInterrupt := range prevInterrupts {
			fmt.Fprintf(buf, infoMetricFmt, irq, prevInterrupt.Devices, prevInterrupt.Info, 0, promTs)
			metricCount += 1
		}
	}
	if buf.Len() > 0 {
		buf.WriteByte('\n')
		byteCount += buf.Len()
	}
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

	// Some housekeeping if delta strategy is in effect:
	if deltaStrategy {
		// Assign a refresh group# to each new irq. To help with testing this
		// should be deterministic, but the new irqs were discovered in hash
		// order (*), so the list will be sorted beforehand.
		// (*): that's still deterministic, technically, but only by reverse
		// engineering the hash function, i.e. by peeking into Go internals.
		if len(newIrqs) > 0 {
			sort.Strings(newIrqs)
			nextRefreshGroupNum := procInterruptsMetricsCtx.nextRefreshGroupNum
			for _, irq := range newIrqs {
				refreshGroupNum[irq] = nextRefreshGroupNum
				nextRefreshGroupNum += 1
				if nextRefreshGroupNum >= fullMetricsFactor {
					nextRefreshGroupNum = 0
				}
			}
			procInterruptsMetricsCtx.nextRefreshGroupNum = nextRefreshGroupNum
			procInterruptsMetricsCtx.newIrqs = newIrqs
		}

		// Check for irqs that were removed, i.e. those left in prevInterrupts:
		if len(prevInterrupts) > 0 {
			for irq := range prevInterrupts {
				delete(refreshGroupNum, irq)
			}
			procInterruptsMetricsCtx.refreshGroupNum = refreshGroupNum
		}

		// Move to the next refresh cycle:
		refreshCycleNum += 1
		if refreshCycleNum >= fullMetricsFactor {
			refreshCycleNum = 0
		}
		procInterruptsMetricsCtx.refreshCycleNum = refreshCycleNum

		// Finally update the prev scan info:
		procInterruptsMetricsCtx.prevInterrupts = interrupts
	}

	// Report the internal metrics stats:
	allMetricsGeneratorInfo.Report(procInterruptsMetricsCtx.generatorId, metricCount, byteCount)
}

var ProcInterruptsMetricsScanIntervalArg = flag.Float64(
	"proc-interrupts-metrics-scan-interval",
	DEFAULT_PROC_INTERRUPTS_METRICS_SCAN_INTERVAL,
	`proc_interrupts metrics interval in seconds, use 0 to disable.`,
)

var ProcInterruptsMetricsFullMetricsIntervalArg = flag.Float64(
	"proc-interrupts-metrics-full-metrics-interval",
	DEFAULT_PROC_INTERRUPTS_METRICS_FULL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often to generate full metrics, in seconds; normally only the metrics
	whose value has changed from the previous scan are generated, but every
	so often the entire set is generated to prevent queries from having to go
	too much back in time to find the last value. Use 0 to generate full
	metrics at every scan.
	`),
)

func BuildProcInterruptsMetricsCtxFromArgs() (*ProcInterruptsMetricsContext, error) {
	if *ProcInterruptsMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*ProcInterruptsMetricsScanIntervalArg * float64(time.Second))
	fullMetricsInterval := time.Duration(*ProcInterruptsMetricsFullMetricsIntervalArg * float64(time.Second))
	fullMetricsFactor := int64(fullMetricsInterval / interval)
	if fullMetricsFactor < 1 {
		fullMetricsFactor = 1
	}

	procInterruptsMetricsCtx, err := NewProcInterruptsMetricsContext(
		interval,
		fullMetricsFactor,
		GlobalProcfsRoot,
		// needed for testing:
		GlobalMetricsHostname,
		GlobalMetricsJob,
		// will be set to default values:
		nil, // timeNow TimeNowFn,
		nil, // wChan chan *bytes.Buffer,
		nil, // bufPool *BufferPool,
	)
	if err != nil {
		return nil, err
	}
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: interval=%s", interval)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: hostname=%s", GlobalMetricsHostname)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: job=%s", GlobalMetricsJob)
	return procInterruptsMetricsCtx, nil
}

var GlobalProcInterruptsMetricsCtx *ProcInterruptsMetricsContext

func StartProcInterruptsMetricsFromArgs() error {
	procInterruptsMetricsCtx, err := BuildProcInterruptsMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procInterruptsMetricsCtx == nil {
		ProcInterruptsMetricsLog.Warn("proc_interrupts metrics collection disabled")
		allMetricsGeneratorInfo.Register(PROC_INTERRUPTS_METRICS_GENERATOR_ID, 0)
		return nil
	}

	allMetricsGeneratorInfo.Register(
		PROC_INTERRUPTS_METRICS_GENERATOR_ID,
		1,
		fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME, procInterruptsMetricsCtx.interval),
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
			time.Duration(*ProcInterruptsMetricsFullMetricsIntervalArg*float64(time.Second)),
		),
		fmt.Sprintf(
			`%s="%d"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME,
			procInterruptsMetricsCtx.fullMetricsFactor,
		),
	)

	GlobalProcInterruptsMetricsCtx = procInterruptsMetricsCtx
	GlobalSchedulerContext.Add(procInterruptsMetricsCtx)
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcInterruptsMetricsFromArgs)
}
