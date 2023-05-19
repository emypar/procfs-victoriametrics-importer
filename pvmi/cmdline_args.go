// Command line argument definitions:

package pvmi

import (
	"bytes"
	"flag"
	"strings"
)

const (
	// proc_interrupts defaults:
	DEFAULT_PROC_INTERRUPTS_METRICS_SCAN_INTERVAL         = 1  // seconds
	DEFAULT_PROC_INTERRUPTS_METRICS_FULL_METRICS_INTERVAL = 15 // seconds

	// proc_softirqs defaults:
	DEFAULT_PROC_SOFTIRQS_METRICS_SCAN_INTERVAL         = "1s"
	DEFAULT_PROC_SOFTIRQS_METRICS_FULL_METRICS_INTERVAL = "15s"

	// The help lines are wrapped around to the following width:
	DEFAULT_USAGE_WIDTH = 58
)

// proc_interrupts:
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

// proc_softirqs:
var ProcSoftirqsMetricsScanIntervalArg = flag.String(
	"proc-softirqs-metrics-scan-interval",
	DEFAULT_PROC_SOFTIRQS_METRICS_SCAN_INTERVAL,
	`proc_softirqs metrics interval in seconds, use 0 to disable.`,
)

var ProcSoftirqsMetricsFullMetricsIntervalArg = flag.String(
	"proc-softirqs-metrics-full-metrics-interval",
	DEFAULT_PROC_SOFTIRQS_METRICS_FULL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often to generate full metrics, in seconds; normally only the metrics
	whose value has changed from the previous scan are generated, but every
	so often the entire set is generated to prevent queries from having to go
	too much back in time to find the last value. Use 0 to generate full
	metrics at every scan.
	`),
)

// Format command flag usage for help message.
func FormatFlagUsageWidth(usage string, width int) string {
	buf := &bytes.Buffer{}
	lineLen := 0
	for _, word := range strings.Fields(strings.TrimSpace(usage)) {
		if lineLen == 0 {
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen = n
		} else {
			if lineLen+len(word)+1 > width {
				buf.WriteByte('\n')
				lineLen = 0
			} else {
				buf.WriteByte(' ')
				lineLen++
			}
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen += n
		}
	}
	return buf.String()
}

func FormatFlagUsage(usage string) string {
	return FormatFlagUsageWidth(usage, DEFAULT_USAGE_WIDTH)
}

type StartGeneratorFromArgsFn func() error

var GlobalStartGeneratorFromArgsList []StartGeneratorFromArgsFn

func RegisterStartGeneratorFromArgs(fn StartGeneratorFromArgsFn) {
	GlobalStartGeneratorFromArgsList = append(GlobalStartGeneratorFromArgsList, fn)
}
