package pvmi

import (
	"bytes"
	"fmt"
	"time"

	"github.com/prometheus/procfs"
)

const (
	// Stats metrics:
	PROC_NET_SNMP_METRICS_GENERATOR_ID = "proc_net_snmp_metrics"
)

var ProcNetSnmpMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcNetSnmpMetrics",
)

// The context for proc interrupts metrics generation:
type ProcNetSnmpMetricsContext struct {
	// Scan interval:
	interval time.Duration
	// Delta / full refresh fields, see `Delta / Full Refresh` section in
	// `metrics.md`.
	fullMetricsInterval time.Duration
	fullMetricsFactor   int
	refreshCycleNum     int
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// The dual object buffer, useful for delta strategy, optionally combined w/
	// re-usable objects.  During the scan object[crtIndex] hold the current
	// state of the object and object[1 - crtIndex], if not nil,  holds the
	// previous one.
	netSnmp          []*procfs.FixedLayoutDataModel
	crtNetSnmpIndex  int
	netSnmp6         []*procfs.FixedLayoutDataModel
	crtNetSnmp6Index int
	// Precomputed metrics, slices parallel with FixedLayoutDataModel.Values:
	netSnmpMetricList  []string
	netSnmp6MetricList []string
	// Internal metrics generator ID:
	generatorId string
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// The following are useful for testing:
	hostname                  string
	job                       string
	timeNow                   TimeNowFn
	bufPool                   *BufferPool
	skipNetSnmp, skipNetSnmp6 bool
}

func (procNetSnmpMetricsCtx *ProcNetSnmpMetricsContext) GetInterval() time.Duration {
	return procNetSnmpMetricsCtx.interval
}

func (procNetSnmpMetricsCtx *ProcNetSnmpMetricsContext) GetGeneratorId() string {
	return procNetSnmpMetricsCtx.generatorId
}

func NewProcNetSnmpMetricsContext(
	interval time.Duration,
	fullMetricsInterval time.Duration,
	procfsRoot string,
	// needed for testing:
	hostname string,
	job string,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	bufPool *BufferPool,
) (*ProcNetSnmpMetricsContext, error) {
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
	procNetSnmpMetricsCtx := &ProcNetSnmpMetricsContext{
		interval:            interval,
		fullMetricsInterval: fullMetricsInterval,
		fullMetricsFactor:   int(fullMetricsInterval / interval),
		refreshCycleNum:     0,
		fs:                  fs,
		netSnmp:             make([]*procfs.FixedLayoutDataModel, 2),
		crtNetSnmpIndex:     1,
		netSnmp6:            make([]*procfs.FixedLayoutDataModel, 2),
		crtNetSnmp6Index:    1,
		generatorId:         PROC_INTERRUPTS_METRICS_GENERATOR_ID,
		wChan:               wChan,
		hostname:            hostname,
		job:                 job,
		timeNow:             timeNow,
		bufPool:             bufPool,
	}
	return procNetSnmpMetricsCtx, nil
}

func (procNetSnmpMetricsCtx *ProcNetSnmpMetricsContext) buildNetSnmpMeticList(fldm *procfs.FixedLayoutDataModel) []string {
	return buildFldmMetricList(fldm, NetSnmpMetricNameMap, procNetSnmpMetricsCtx.hostname, procNetSnmpMetricsCtx.job)
}

func (procNetSnmpMetricsCtx *ProcNetSnmpMetricsContext) buildNetSnmp6MeticList(fldm *procfs.FixedLayoutDataModel) []string {
	return buildFldmMetricList(fldm, NetSnmp6MetricNameMap, procNetSnmpMetricsCtx.hostname, procNetSnmpMetricsCtx.job)
}

func (procNetSnmpMetricsCtx *ProcNetSnmpMetricsContext) GenerateMetrics() {
	var (
		err              error
		deltaMetricCount int
		crtIndex         int
	)

	fs := procNetSnmpMetricsCtx.fs
	bufPool := procNetSnmpMetricsCtx.bufPool
	wChan := procNetSnmpMetricsCtx.wChan
	fullMetricsFactor := procNetSnmpMetricsCtx.fullMetricsFactor
	refreshCycleNum := procNetSnmpMetricsCtx.refreshCycleNum
	timeNow := procNetSnmpMetricsCtx.timeNow

	buf := bufPool.GetBuffer()
	metricCount := 0

	if !procNetSnmpMetricsCtx.skipNetSnmp {
		crtIndex = 1 - procNetSnmpMetricsCtx.crtNetSnmpIndex
		deltaMetricCount, err = GenerateFldmDataModelMetrics(
			procNetSnmpMetricsCtx.netSnmp,
			crtIndex,
			fs.NetSnmpDataModel,
			&procNetSnmpMetricsCtx.netSnmpMetricList,
			procNetSnmpMetricsCtx.buildNetSnmpMeticList,
			fullMetricsFactor,
			refreshCycleNum,
			timeNow,
			buf,
		)
		if err != nil {
			ProcNetSnmpMetricsLog.Warn(err)
		} else {
			metricCount += deltaMetricCount
			procNetSnmpMetricsCtx.crtNetSnmpIndex = crtIndex
		}
	}

	if !procNetSnmpMetricsCtx.skipNetSnmp6 {
		crtIndex = 1 - procNetSnmpMetricsCtx.crtNetSnmp6Index
		deltaMetricCount, err = GenerateFldmDataModelMetrics(
			procNetSnmpMetricsCtx.netSnmp6,
			crtIndex,
			fs.NetSnmp6DataModel,
			&procNetSnmpMetricsCtx.netSnmp6MetricList,
			procNetSnmpMetricsCtx.buildNetSnmp6MeticList,
			fullMetricsFactor,
			refreshCycleNum,
			timeNow,
			buf,
		)
		if err != nil {
			ProcNetSnmpMetricsLog.Warn(err)
		} else {
			metricCount += deltaMetricCount
			procNetSnmpMetricsCtx.crtNetSnmp6Index = crtIndex
		}
	}

	byteCount := buf.Len()
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

	if fullMetricsFactor > 1 {
		if refreshCycleNum++; refreshCycleNum >= fullMetricsFactor {
			refreshCycleNum = 0
		}
		procNetSnmpMetricsCtx.refreshCycleNum = refreshCycleNum
	}

	allMetricsGeneratorInfo.Report(procNetSnmpMetricsCtx.generatorId, metricCount, byteCount)
}

func BuildProcNetSnmpMetricsCtxFromArgs() (*ProcNetSnmpMetricsContext, error) {
	if *ProcNetSnmpScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*ProcNetSnmpScanIntervalArg * float64(time.Second))
	fullMetricsInterval := time.Duration(*ProcNetSnmpFullMetricsIntervalArg * float64(time.Second))
	procNetSnmpMetricsCtx, err := NewProcNetSnmpMetricsContext(
		interval,
		fullMetricsInterval,
		GlobalProcfsRoot,
		// needed for testing, will be set to default values:
		"",
		"",
		nil, // timeNow TimeNowFn,
		nil, // wChan chan *bytes.Buffer,
		nil, // bufPool *BufferPool,

	)
	if err != nil {
		return nil, err
	}
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: interval=%s", procNetSnmpMetricsCtx.interval)
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: fullMetricsInterval=%s", procNetSnmpMetricsCtx.fullMetricsInterval)
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: fullMetricsFactor=%d", procNetSnmpMetricsCtx.fullMetricsFactor)
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: hostname=%s", procNetSnmpMetricsCtx.hostname)
	ProcNetSnmpMetricsLog.Infof("proc_net_snmp metrics: job=%s", procNetSnmpMetricsCtx.job)
	return procNetSnmpMetricsCtx, nil
}

func StartProcNetSnmpMetricsFromArgs() error {
	procNetSnmpMetricsCtx, err := BuildProcNetSnmpMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procNetSnmpMetricsCtx == nil {
		ProcNetSnmpMetricsLog.Warn("proc_net_snmp metrics collection disabled")
		allMetricsGeneratorInfo.Register(PROC_INTERRUPTS_METRICS_GENERATOR_ID, 0)
		return nil
	}

	allMetricsGeneratorInfo.Register(
		PROC_NET_SNMP_METRICS_GENERATOR_ID,
		1,
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_INTERVAL_LABEL_NAME,
			procNetSnmpMetricsCtx.interval,
		),
		fmt.Sprintf(
			`%s="%s"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_INTERVAL_LABEL_NAME,
			procNetSnmpMetricsCtx.fullMetricsInterval,
		),
		fmt.Sprintf(
			`%s="%d"`,
			INTERNAL_METRICS_GENERATOR_CONFIG_FULL_METRICS_FACTOR_LABEL_NAME,
			procNetSnmpMetricsCtx.fullMetricsFactor,
		),
	)

	GlobalSchedulerContext.Add(procNetSnmpMetricsCtx)
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcNetSnmpMetricsFromArgs)
}
