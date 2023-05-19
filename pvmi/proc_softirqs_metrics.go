package pvmi

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	// Use for the auto-detection of the number of cpus to keep (/proc/softirqs
	// may report more than actually exist):
	PROC_SOFTIRQS_KEEP_NUM_CPUS_AUTODETECT = -1

	// Stats metrics:
	PROC_SOFTIRQS_METRICS_GENERATOR_ID = "proc_softirqs_metrics"
)

var ProcSoftirqsMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcSoftirqsMetrics",
)

type ProcSoftirqsMetricsContext struct {
	// Interval:
	interval time.Duration
	// Full metrics factor, N. For delta strategy, metrics are generated only
	// for stats that changed from the previous run, however the full set is
	// generated at every Nth pass. Use  N = 1 for full metrics every time.
	fullMetricsFactor int64
	// The refresh cycle#, modulo fullMetricsFactor. Metrics are bundled
	// together in groups with group# (the CPU# is used for group#) also modulo
	// fullMetricsFactor. Each time group# == refresh cycle, all the metrics in
	// the group are generated regardless whether they changed or not from the
	// previous scan:
	refreshCycleNum int64
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// Use the dual object approach for the parsed data; at the end of the scan
	// the current object is pointed by crtIndex:
	softirqs [2]*procfs.Softirqs
	crtIndex int
	// /proc/softirqs may have more columns that actual CPUs. Limit the parsing
	// to the actual useful data (0 means no cap):
	keepNumCpus int
	// Precomputed formats for generating the metrics:
	counterMetricFmt string
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

func (procSoftirqsMetricsCtx *ProcSoftirqsMetricsContext) GetInterval() time.Duration {
	return procSoftirqsMetricsCtx.interval
}

func (procSoftirqsMetricsCtx *ProcSoftirqsMetricsContext) GetGeneratorId() string {
	return procSoftirqsMetricsCtx.generatorId
}

func (procSoftirqsMetricsCtx *ProcSoftirqsMetricsContext) GenerateMetrics() {
	var (
		err error
	)

	metricCount, byteCount := 0, 0
	savedCrtIndex := procSoftirqsMetricsCtx.crtIndex
	defer func() {
		if err != nil {
			procSoftirqsMetricsCtx.crtIndex = savedCrtIndex
		}

	}()

	prevSoftirqs := procSoftirqsMetricsCtx.softirqs[procSoftirqsMetricsCtx.crtIndex]
	procSoftirqsMetricsCtx.crtIndex = 1 - procSoftirqsMetricsCtx.crtIndex
	softirqs := procSoftirqsMetricsCtx.softirqs[procSoftirqsMetricsCtx.crtIndex]
	if softirqs == nil {
		softirqs = &procfs.Softirqs{}
		procSoftirqsMetricsCtx.softirqs[procSoftirqsMetricsCtx.crtIndex] = softirqs
	}

	_, err = procSoftirqsMetricsCtx.fs.SoftirqsWithStruct(softirqs, procSoftirqsMetricsCtx.keepNumCpus)
	if err != nil {
		ProcSoftirqsMetricsLog.Warn(err)
		return
	}
	timestamp := procSoftirqsMetricsCtx.timeNow().UTC()
	promTs := strconv.FormatInt(timestamp.UnixMilli(), 10)

	bufPool := procSoftirqsMetricsCtx.bufPool
	wChan := procSoftirqsMetricsCtx.wChan
	buf := bufPool.GetBuffer()

	refreshCycleNum, fullMetricsFactor := procSoftirqsMetricsCtx.refreshCycleNum, procSoftirqsMetricsCtx.fullMetricsFactor
	counterMetricFmt := procSoftirqsMetricsCtx.counterMetricFmt

	generateIrqMetrics := func(irq string, cpuCount, prevCpuCount []uint64) {
		var groupNum int64 = 0
		for cpu, count := range cpuCount {
			if prevCpuCount == nil || groupNum == refreshCycleNum || prevCpuCount[cpu] != count {
				fmt.Fprintf(buf, counterMetricFmt, irq, cpu, count, promTs)
				metricCount += 1
			}
			if groupNum += 1; groupNum >= fullMetricsFactor {
				groupNum = 0
			}
		}
	}

	if fullMetricsFactor <= 1 || prevSoftirqs == nil {
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_HI, softirqs.Hi, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_TIMER, softirqs.Timer, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_NET_TX, softirqs.NetTx, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_NET_RX, softirqs.NetRx, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_BLOCK, softirqs.Block, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_IRQ_POLL, softirqs.IRQPoll, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_TASKLET, softirqs.Tasklet, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_SCHED, softirqs.Sched, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_HR_TIMER, softirqs.HRTimer, nil)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_RCU, softirqs.RCU, nil)
	} else {
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_HI, softirqs.Hi, prevSoftirqs.Hi)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_TIMER, softirqs.Timer, prevSoftirqs.Timer)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_NET_TX, softirqs.NetTx, prevSoftirqs.NetTx)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_NET_RX, softirqs.NetRx, prevSoftirqs.NetRx)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_BLOCK, softirqs.Block, prevSoftirqs.Block)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_IRQ_POLL, softirqs.IRQPoll, prevSoftirqs.IRQPoll)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_TASKLET, softirqs.Tasklet, prevSoftirqs.Tasklet)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_SCHED, softirqs.Sched, prevSoftirqs.Sched)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_HR_TIMER, softirqs.HRTimer, prevSoftirqs.HRTimer)
		generateIrqMetrics(PROC_SOFTIRQS_TOTAL_IRQ_LABEL_VALUE_RCU, softirqs.RCU, prevSoftirqs.RCU)
	}

	if refreshCycleNum += 1; refreshCycleNum >= fullMetricsFactor {
		refreshCycleNum = 0
	}
	procSoftirqsMetricsCtx.refreshCycleNum = refreshCycleNum

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
	allMetricsGeneratorInfo.Report(procSoftirqsMetricsCtx.generatorId, metricCount, byteCount)
}

func NewProcSoftirqsMetricsContext(
	interval time.Duration,
	fullMetricsFactor int64,
	procfsRoot string,
	keepNumCpus int,
	// needed for testing:
	hostname string,
	job string,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	bufPool *BufferPool,
) (*ProcSoftirqsMetricsContext, error) {
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
	if keepNumCpus == PROC_SOFTIRQS_KEEP_NUM_CPUS_AUTODETECT {
		stat, err := fs.Stat2(nil)
		if err != nil {
			ProcSoftirqsMetricsLog.Warnf("Cannot autodetect CPU#: %v", err)
			keepNumCpus = 0
		} else {
			keepNumCpus = len(stat.CPU) - 1 // the slice includes `all'
		}
	}
	procSoftirqsMetricsCtx := &ProcSoftirqsMetricsContext{
		interval:          interval,
		fullMetricsFactor: fullMetricsFactor,
		fs:                fs,
		crtIndex:          1,
		keepNumCpus:       keepNumCpus,
		counterMetricFmt: fmt.Sprintf(
			`%s{%s="%s",%s="%s",%s="%%s",%s="%%d"} %%d %%s`+"\n",
			PROC_SOFTIRQS_TOTAL_METRIC_NAME,
			HOSTNAME_LABEL_NAME, hostname,
			JOB_LABEL_NAME, job,
			PROC_SOFTIRQS_TOTAL_IRQ_LABEL_NAME,
			PROC_SOFTIRQS_TOTAL_CPU_LABEL_NAME,
		),
		generatorId: PROC_SOFTIRQS_METRICS_GENERATOR_ID,
		wChan:       wChan,
		hostname:    hostname,
		job:         job,
		timeNow:     timeNow,
		bufPool:     bufPool,
	}
	return procSoftirqsMetricsCtx, nil
}

func BuildProcSoftirqsMetricsCtxFromArgs() (*ProcSoftirqsMetricsContext, error) {
	interval, err := time.ParseDuration(*ProcSoftirqsMetricsScanIntervalArg)
	if err != nil {
		return nil, fmt.Errorf("invalid softirqs scan interval: %v", err)
	}
	if interval <= 0 {
		return nil, nil
	}
	fullMetricsInterval, err := time.ParseDuration(*ProcSoftirqsMetricsFullMetricsIntervalArg)
	if err != nil {
		return nil, fmt.Errorf("invalid softirqs full metrivs interval: %v", err)
	}
	fullMetricsFactor := int64(fullMetricsInterval / interval)
	if fullMetricsFactor < 1 {
		fullMetricsFactor = 1
	}
	procSoftirqsMetricsCtx, err := NewProcSoftirqsMetricsContext(
		interval,
		fullMetricsFactor,
		GlobalProcfsRoot,
		PROC_SOFTIRQS_KEEP_NUM_CPUS_AUTODETECT,
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
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: interval=%s", procSoftirqsMetricsCtx.interval)
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: fullMetricsFactor=%d", procSoftirqsMetricsCtx.fullMetricsFactor)
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: keepNumCpus=%d", procSoftirqsMetricsCtx.keepNumCpus)
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: hostname=%s", GlobalMetricsHostname)
	ProcSoftirqsMetricsLog.Infof("proc_softirqs metrics: job=%s", GlobalMetricsJob)

	return procSoftirqsMetricsCtx, nil
}

func StartProcSoftirqsMetricsFromArgs() error {
	procSoftirqsMetricsCtx, err := BuildProcSoftirqsMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procSoftirqsMetricsCtx == nil {
		ProcSoftirqsMetricsLog.Warn("proc_softirqs metrics collection disabled")
		allMetricsGeneratorInfo.Register(PROC_SOFTIRQS_METRICS_GENERATOR_ID, 0)
		return nil
	}
	allMetricsGeneratorInfo.Register(
		PROC_SOFTIRQS_METRICS_GENERATOR_ID,
		1,
		fmt.Sprintf(`%s="%s"`, INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME, procSoftirqsMetricsCtx.interval),
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
			*ProcSoftirqsMetricsFullMetricsIntervalArg,
		),
		fmt.Sprintf(
			`%s="%d"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME,
			procSoftirqsMetricsCtx.fullMetricsFactor,
		),
	)
	GlobalSchedulerContext.Add(procSoftirqsMetricsCtx)
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcSoftirqsMetricsFromArgs)
}
