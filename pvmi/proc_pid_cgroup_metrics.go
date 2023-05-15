// proc_pid_cgroup metrics utils.

package pvmi

import (
	"bytes"
	"fmt"

	"github.com/prometheus/procfs"
)

const (
	PROC_PID_CGROUP_METRIC_NAME                    = "proc_pid_cgroup"
	PROC_PID_CGROUP_METRIC_HIERARCHY_ID_LABEL_NAME = "hierarchy_id"
	PROC_PID_CGROUP_METRIC_CONTROLLER_LABEL_NAME   = "controller"
	PROC_PID_CGROUP_METRIC_PATH_LABEL_NAME         = "path"
)

var pidCgroupMetricFmt = fmt.Sprintf(
	`%s{%%s,%s="%%d",%s="%%s",%s="%%s"}`,
	PROC_PID_CGROUP_METRIC_NAME,
	PROC_PID_CGROUP_METRIC_HIERARCHY_ID_LABEL_NAME,
	PROC_PID_CGROUP_METRIC_CONTROLLER_LABEL_NAME,
	PROC_PID_CGROUP_METRIC_PATH_LABEL_NAME,
)

// Invoke for new/changed cgroup info:
func updateProcPidCgroupMetric(
	pmce *PidMetricsCacheEntry,
	promTs string,
	buf *bytes.Buffer,
	metricCount *int,
) error {
	cgroups, err := procfs.ParseCgroups(pmce.RawCgroup)
	if err != nil {
		return fmt.Errorf("rawCgroup=%q %v: %s", pmce.RawCgroup, pmce.RawCgroup, err)
	}
	// Build the lists of new metrics and of those that went out-of-scope:
	outOfScopeProcPidCgroupMetrics := pmce.ProcPidCgroupMetrics
	pmce.ProcPidCgroupMetrics = map[string]bool{}
	for _, cgroup := range cgroups {
		controllers := cgroup.Controllers
		if len(controllers) == 0 {
			controllers = []string{""}
		}
		for _, controller := range controllers {
			metric := fmt.Sprintf(
				pidCgroupMetricFmt,
				pmce.CommonLabels,
				cgroup.HierarchyID,
				controller,
				cgroup.Path,
			)
			pmce.ProcPidCgroupMetrics[metric] = true
			delete(outOfScopeProcPidCgroupMetrics, metric) // i.e. still valid
		}
	}
	clearSuffix := []byte(" 0 " + promTs + "\n")
	clearMetricCount := 0
	for metric, _ := range outOfScopeProcPidCgroupMetrics {
		buf.WriteString(metric)
		buf.Write(clearSuffix)
		clearMetricCount += 1
	}
	if metricCount != nil {
		*metricCount += clearMetricCount
	}
	return nil
}
