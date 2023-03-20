// proc_pid_cmdline metrics utils.

package pvmi

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	// proc_pid_cmdline:
	PROC_PID_CMDLINE_METRIC_NAME               = "proc_pid_cmdline"
	PROC_PID_CMDLINE_METRIC_CMDLINE_LABEL_NAME = "cmdline"
)

var pidCmdlineMetricFmt = fmt.Sprintf(
	`%s{%%s,%s="%%s"}`,
	PROC_PID_CMDLINE_METRIC_NAME,
	PROC_PID_CMDLINE_METRIC_CMDLINE_LABEL_NAME,
)

// Invoke for new/changed cmdline:
func updateProcPidCmdlineMetric(
	pmce *PidMetricsCacheEntry,
	rawCmdline []byte,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	// Clear previous metric, if any:
	if pmce.ProcPidCmdlineMetric != "" {
		buf.WriteString(pmce.ProcPidCmdlineMetric + " 0 " + promTs + "\n")
	}
	// New metric:
	nullByte := []byte{0}
	spaceByte := []byte{' '}
	cmdline := strings.TrimSpace(
		string(
			bytes.ReplaceAll(
				bytes.TrimSuffix(rawCmdline, nullByte),
				nullByte,
				spaceByte,
			),
		),
	)
	pmce.ProcPidCmdlineMetric = fmt.Sprintf(
		pidCmdlineMetricFmt,
		pmce.CommonLabels,
		SanitizeLabelValue(cmdline),
	)
	if generatedCount != nil {
		*generatedCount += 1
	}
	return nil
}
