// common metrics functions

package pvmi

import (
	"bytes"
	"strconv"
)

const (
	METRICS_WRITE_CHANNEL_BASED_ON_BUF_POOL_SIZE = -1
	DEFAULT_METRICS_WRITE_CHANNEL_SIZE           = METRICS_WRITE_CHANNEL_BASED_ON_BUF_POOL_SIZE
	METRICS_WRITE_CHANNEL_MIN_SIZE               = 16
)

// Sanitize label values as per
// https://github.com/Showmax/prometheus-docs/blob/master/content/docs/instrumenting/exposition_formats.md
func SanitizeLabelValue(v string) string {
	qVal := strconv.Quote(v)
	// Remove enclosing `"':
	return qVal[1 : len(qVal)-1]
}

func NewMetricsWriteChannelFromArgs() chan *bytes.Buffer {
	metricsWriteChanSize := *MetricsWriteChanSizeArg
	if metricsWriteChanSize <= 0 {
		metricsWriteChanSize = *BufPoolMaxSizeArg * 9 / 10
	}
	if metricsWriteChanSize < METRICS_WRITE_CHANNEL_MIN_SIZE {
		metricsWriteChanSize = METRICS_WRITE_CHANNEL_MIN_SIZE
	}
	return make(chan *bytes.Buffer, metricsWriteChanSize)
}

var GlobalMetricsWriteChannel chan *bytes.Buffer

func SetGlobalMetricsWriteChannelFromArgs() {
	GlobalMetricsWriteChannel = NewMetricsWriteChannelFromArgs()
}
