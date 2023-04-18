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

func GenerateInternalMetrics(mGenCtx MetricsGenContext) {
	internalMetricsCtx := mGenCtx.(*InternalMetricsContext)
	wChan := internalMetricsCtx.wChan
	bufPool := internalMetricsCtx.bufPool
	hostname, job := internalMetricsCtx.hostname, internalMetricsCtx.job
	buf := bufPool.GetBuffer()

	timestampStr := strconv.FormatInt(time.Now().UnixMilli(), 10)

	fmt.Fprintf(
		buf,
		`%s{%s="%s",%s="%s"} %d %s`+"\n",
		INTERNAL_GOROUTINE_COUNT_METRIC_NAME,
		HOSTNAME_LABEL_NAME, hostname,
		JOB_LABEL_NAME, job,
		runtime.NumGoroutine(), timestampStr,
	)

	if wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}
}

func init() {
	RegisterStartGeneratorFromArgs(StartInternalMetricsFromArgs)
}
