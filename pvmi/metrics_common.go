// common metrics functions

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/prometheus/procfs"
)

const (
	DEFAULT_METRICS_WRITE_CHANNEL_SIZE = -1
	METRICS_WRITE_CHANNEL_MIN_SIZE     = 16

	DEFAULT_PROCFS_ROOT = "/proc"

	// Certain metrics have values that would exceed the 15 (15.95) precision of
	// `float64`, e.g. `uint64`. Such values will be split in two metrics,
	// `..._low32` and `..._high32` where `low32` == `value & 0xffffffff` and
	// `high32` = `value >> 32`. This is useful for deltas or rates where the
	// operation is applied 1st to each of the 2 halves which are then combined.
	// e.g.: `delta = delta(high32) * 4294967296 + delta(low32)`, with the
	// underlying assumption that the delta is fairly small. This is how the
	// byte count is handled for interfaces. Define the distinguished suffixes
	// for the low and high halves:
	LOW32_METRICS_NAME_SUFFIX  = "_low32"
	HIGH32_METRICS_NAME_SUFFIX = "_high32"

	// All metrics will have the following labels:
	HOSTNAME_LABEL_NAME     = "hostname"
	JOB_LABEL_NAME          = "job"
	DEFAULT_JOB_LABEL_VALUE = "pvmi"
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

var MetricsJobLabelValueArgs = flag.String(
	"metrics-job",
	DEFAULT_JOB_LABEL_VALUE,
	FormatFlagUsage(fmt.Sprintf(`
	Set the value to use for %s label, common to all metrics.
	`, JOB_LABEL_NAME)),
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

var GlobalProcfsRoot string
var GlobalMetricsHostname string
var GlobalMetricsJob string

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

var commonMetricsLabelValuesSet = false

func SetCommonMetricsPreRequisitesFromArgs() error {
	if commonMetricsLabelValuesSet {
		return nil
	}
	_, err := procfs.NewFS(*MetricsProcfsRootArg)
	if err != nil {
		return err
	}
	GlobalProcfsRoot = *MetricsProcfsRootArg

	err = SetMetricsHostnameFromArgs()
	if err != nil {
		return err
	}
	GlobalMetricsJob = *MetricsJobLabelValueArgs
	commonMetricsLabelValuesSet = true
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
