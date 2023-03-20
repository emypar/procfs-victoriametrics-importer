// proc_pid_cgroup metrics utils.

package pvmi

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

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
	rawCgroup []byte,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	cgroups, err := parseCgroups(rawCgroup)
	if err != nil {
		return err
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
	gCnt := uint64(0)
	for metric, _ := range outOfScopeProcPidCgroupMetrics {
		buf.WriteString(metric)
		buf.Write(clearSuffix)
		gCnt += 1
	}
	if generatedCount != nil {
		*generatedCount += gCnt
	}
	return nil
}

// from "github.com/prometheus/procfs", parse cgroups from data, rather than proc

// parseCgroupString parses each line of the /proc/[pid]/cgroup file
// Line format is hierarchyID:[controller1,controller2]:path.
func parseCgroupString(cgroupStr string) (*procfs.Cgroup, error) {
	var err error

	fields := strings.SplitN(cgroupStr, ":", 3)
	if len(fields) < 3 {
		return nil, fmt.Errorf("at least 3 fields required, found %d fields in cgroup string: %s", len(fields), cgroupStr)
	}

	cgroup := &procfs.Cgroup{
		Path:        fields[2],
		Controllers: nil,
	}
	cgroup.HierarchyID, err = strconv.Atoi(fields[0])
	if err != nil {
		return nil, fmt.Errorf("failed to parse hierarchy ID")
	}
	if fields[1] != "" {
		ssNames := strings.Split(fields[1], ",")
		cgroup.Controllers = append(cgroup.Controllers, ssNames...)
	}
	return cgroup, nil
}

// parseCgroups reads each line of the /proc/[pid]/cgroup file.
func parseCgroups(data []byte) ([]procfs.Cgroup, error) {
	var cgroups []procfs.Cgroup
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		mountString := scanner.Text()
		parsedMounts, err := parseCgroupString(mountString)
		if err != nil {
			return nil, err
		}
		cgroups = append(cgroups, *parsedMounts)
	}

	err := scanner.Err()
	return cgroups, err
}
