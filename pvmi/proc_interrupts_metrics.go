// /proc/interrupts

package pvmi

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	// Stats metrics:
	PROC_INTERRUPTS_METRICS_GENERATOR_ID = "proc_interrupts_metrics"
)

var ProcInterruptsMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcInterruptsMetrics",
)

// The context for proc interrupts metrics generation:
type ProcInterruptsMetricsContext struct {
	// Scan interval:
	interval time.Duration
	// Delta / full refresh fields, see `Delta / Full Refresh` section in
	// `metrics.md`.
	fullMetricsInterval time.Duration
	fullMetricsFactor   int
	refreshCycleNum     int
	nextRefreshGroupNum int
	refreshGroupNum     map[string]int
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// The dual object buffer, useful for delta strategy, optionally combined w/
	// re-usable objects.  During the scan object[crtIndex] hold the current
	// state of the object and object[1 - crtIndex], if not nil,  holds the
	// previous one.
	interrupts [2]procfs.Interrupts
	crtIndex   int
	// Precomputed formats to speed up the metrics generation:
	interruptsCounterMetricFmt string
	interruptsInfoMetricFmt    string
	// Internal metrics generator ID:
	generatorId string
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// The following are useful for testing:
	hostname                     string
	job                          string
	timeNow                      TimeNowFn
	bufPool                      *BufferPool
	skipInterrupts, skipSoftirqs bool
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GetInterval() time.Duration {
	return procInterruptsMetricsCtx.interval
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GetGeneratorId() string {
	return procInterruptsMetricsCtx.generatorId
}

func (procInterruptsMetricsCtx *ProcInterruptsMetricsContext) GetRefreshGroupNum(objId string) int {
	refreshGroupNum, ok := procInterruptsMetricsCtx.refreshGroupNum[objId]
	if !ok {
		refreshGroupNum = procInterruptsMetricsCtx.nextRefreshGroupNum
		procInterruptsMetricsCtx.refreshGroupNum[objId] = refreshGroupNum
		procInterruptsMetricsCtx.nextRefreshGroupNum = refreshGroupNum + 1
		if procInterruptsMetricsCtx.nextRefreshGroupNum >= procInterruptsMetricsCtx.fullMetricsFactor {
			procInterruptsMetricsCtx.nextRefreshGroupNum = 0
		}
	}
	return refreshGroupNum
}

func NewProcInterruptsMetricsContext(
	interval time.Duration,
	fullMetricsInterval time.Duration,
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
		fullMetricsInterval: fullMetricsInterval,
		fullMetricsFactor:   int(fullMetricsInterval / interval),
		refreshCycleNum:     0,
		refreshGroupNum:     make(map[string]int),
		nextRefreshGroupNum: 0,
		fs:                  fs,
		crtIndex:            1,
		interruptsCounterMetricFmt: fmt.Sprintf(
			`%s{%s="%s",%s="%s",%s="%%s",%s="%%d"} %%s %%s`+"\n",
			PROC_INTERRUPTS_TOTAL_METRIC_NAME,
			HOSTNAME_LABEL_NAME, hostname,
			JOB_LABEL_NAME, job,
			PROC_INTERRUPTS_IRQ_LABEL_NAME,
			PROC_INTERRUPTS_TOTAL_CPU_LABEL_NAME,
		),
		interruptsInfoMetricFmt: fmt.Sprintf(
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
	var (
		err        error
		interrupts procfs.Interrupts
	)

	savedCrtIndex := procInterruptsMetricsCtx.crtIndex
	defer func() {
		if err != nil {
			procInterruptsMetricsCtx.crtIndex = savedCrtIndex
		}
	}()

	procInterruptsMetricsCtx.crtIndex = 1 - procInterruptsMetricsCtx.crtIndex

	skipInterrupts := procInterruptsMetricsCtx.skipInterrupts
	if !skipInterrupts {
		interrupts, err = procInterruptsMetricsCtx.fs.Interrupts()
		if err != nil {
			ProcInterruptsMetricsLog.Warn(err)
			return
		}
		procInterruptsMetricsCtx.interrupts[procInterruptsMetricsCtx.crtIndex] = interrupts
	}
	promTs := strconv.FormatInt(procInterruptsMetricsCtx.timeNow().UnixMilli(), 10)

	metricCount, byteCount := 0, 0

	bufPool := procInterruptsMetricsCtx.bufPool
	buf := bufPool.GetBuffer()
	wChan := procInterruptsMetricsCtx.wChan

	fullMetricsFactor := procInterruptsMetricsCtx.fullMetricsFactor
	refreshCycleNum := procInterruptsMetricsCtx.refreshCycleNum

	if !skipInterrupts {
		prevInterrupts := procInterruptsMetricsCtx.interrupts[1-procInterruptsMetricsCtx.crtIndex]
		interruptsCounterMetricFmt := procInterruptsMetricsCtx.interruptsCounterMetricFmt
		interruptsInfoMetricFmt := procInterruptsMetricsCtx.interruptsInfoMetricFmt

		for irq, interrupt := range interrupts {
			var (
				prevInterrupt procfs.Interrupt
				exists        bool
			)
			fullMetrics := fullMetricsFactor <= 1 || procInterruptsMetricsCtx.GetRefreshGroupNum(irq) == refreshCycleNum
			if !fullMetrics {
				prevInterrupt, exists = prevInterrupts[irq]
				if !exists {
					fullMetrics = true
				}
			}
			// Handle pseudo-categorical info 1st:
			if exists && (prevInterrupt.Devices != interrupt.Devices || prevInterrupt.Info != interrupt.Info) {
				fmt.Fprintf(buf, interruptsInfoMetricFmt, irq, prevInterrupt.Devices, prevInterrupt.Info, 0, promTs)
				metricCount += 1
			}
			if fullMetrics {
				fmt.Fprintf(buf, interruptsInfoMetricFmt, irq, interrupt.Devices, interrupt.Info, 1, promTs)
				metricCount += 1
			}
			// Values:
			for cpu, value := range interrupt.Values {
				if fullMetrics || prevInterrupt.Values[cpu] != value {
					fmt.Fprintf(buf, interruptsCounterMetricFmt, irq, cpu, value, promTs)
					metricCount += 1
				}
			}
			// Drop it from prevInterrupts as a way of detecting out-of-scope irq's:
			// what is left in prevInterrupts has to be cleared. This may be an
			// overkill since interrupt sources do not vanish on the fly.
			delete(prevInterrupts, irq)
		}
		if len(prevInterrupts) > 0 {
			refreshGroupNum := procInterruptsMetricsCtx.refreshGroupNum
			for irq, prevInterrupt := range prevInterrupts {
				fmt.Fprintf(buf, interruptsInfoMetricFmt, irq, prevInterrupt.Devices, prevInterrupt.Info, 0, promTs)
				metricCount += 1
				delete(refreshGroupNum, irq)
			}
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
	if fullMetricsFactor > 1 {
		// Move to the next refresh cycle:
		refreshCycleNum += 1
		if refreshCycleNum >= fullMetricsFactor {
			refreshCycleNum = 0
		}
		procInterruptsMetricsCtx.refreshCycleNum = refreshCycleNum
	}

	// Report the internal metrics stats:
	allMetricsGeneratorInfo.Report(procInterruptsMetricsCtx.generatorId, metricCount, byteCount)
}

func BuildProcInterruptsMetricsCtxFromArgs() (*ProcInterruptsMetricsContext, error) {
	if *ProcInterruptsMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*ProcInterruptsMetricsScanIntervalArg * float64(time.Second))
	fullMetricsInterval := time.Duration(*ProcInterruptsMetricsFullMetricsIntervalArg * float64(time.Second))
	procInterruptsMetricsCtx, err := NewProcInterruptsMetricsContext(
		interval,
		fullMetricsInterval,
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
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: interval=%s", procInterruptsMetricsCtx.interval)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: fullMetricsInterval=%s", procInterruptsMetricsCtx.fullMetricsInterval)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: fullMetricsFactor=%d", procInterruptsMetricsCtx.fullMetricsFactor)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: hostname=%s", procInterruptsMetricsCtx.hostname)
	ProcInterruptsMetricsLog.Infof("proc_interrupts metrics: job=%s", procInterruptsMetricsCtx.job)
	return procInterruptsMetricsCtx, nil
}

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
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME,
			procInterruptsMetricsCtx.interval,
		),
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
			procInterruptsMetricsCtx.fullMetricsInterval,
		),
		fmt.Sprintf(
			`%s="%d"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME,
			procInterruptsMetricsCtx.fullMetricsFactor,
		),
	)

	GlobalSchedulerContext.Add(procInterruptsMetricsCtx)
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcInterruptsMetricsFromArgs)
}
