// common metrics functions

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	DEFAULT_METRICS_WRITE_CHANNEL_SIZE = -1
	METRICS_WRITE_CHANNEL_MIN_SIZE     = 16

	DEFAULT_PROCFS_ROOT = "/proc"

	// All metrics will have the following labels:
	HOSTNAME_LABEL_NAME        = "hostname"
	SOURCE_LABEL_NAME          = "source"
	DEFAULT_SOURCE_LABEL_VALUE = "pvmi"
)

var MetricsProcfsRootArg = flag.String(
	"procfs-root",
	DEFAULT_PROCFS_ROOT,
	`Procfs root`,
)

var HostnameArg = flag.String(
	"metrics-hostname",
	"",
	FormatFlagUsage(fmt.Sprintf(`
	Set the value to use for %s label, if different than OS's hostname.
	`, HOSTNAME_LABEL_NAME)),
)

var UseShortHostnameArg = flag.Bool(
	"metrics-use-short-hostname",
	false,
	`Strip the domain from OS's hostname.`,
)

var MetricsSourceLabelValueArgs = flag.String(
	"metrics-source",
	DEFAULT_SOURCE_LABEL_VALUE,
	FormatFlagUsage(fmt.Sprintf(`
	Set the value to use for %s label, common to all metrics.
	`, SOURCE_LABEL_NAME)),
)

var MetricsWriteChanSizeArg = flag.Int(
	"metrics-write-channel-size",
	DEFAULT_METRICS_WRITE_CHANNEL_SIZE,
	FormatFlagUsage(fmt.Sprintf(`
	The size of the metrics write channel (AKA Compressor Queue). Use -1 to
	base it on buffer-pool-max-size arg, 90%% of that value. The value will
	be adjusted to be at least %d.
	`,
		METRICS_WRITE_CHANNEL_MIN_SIZE,
	)),
)

var GlobalMetricsHostname string
var GlobalMetricsSource string

func SetMetricsHostnameFromArgs() error {
	if *HostnameArg != "" {
		GlobalMetricsHostname = *HostnameArg
		return nil
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	if *UseShortHostnameArg {
		i := strings.Index(hostname, ".")
		if i > 0 {
			hostname = hostname[:i]
		}
	}
	GlobalMetricsHostname = hostname
	return nil
}

func SetCommonMetricsLabelValuesFromArgs() error {
	err := SetMetricsHostnameFromArgs()
	if err != nil {
		return err
	}
	GlobalMetricsSource = *MetricsSourceLabelValueArgs
	return nil
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

// Sanitize label values as per
// https://github.com/Showmax/prometheus-docs/blob/master/content/docs/instrumenting/exposition_formats.md
func SanitizeLabelValue(v string) string {
	qVal := strconv.Quote(v)
	// Remove enclosing `"':
	return qVal[1 : len(qVal)-1]
}
