// proc_pid_status_* metrics utils.

package pvmi

import (
	"bytes"
	"fmt"

	"github.com/prometheus/procfs"
)

const (
	PROC_PID_STATUS_INFO_METRIC_NAME               = "proc_pid_status_info"
	PROC_PID_STATUS_INFO_NAME_LABEL_NAME           = "name"
	PROC_PID_STATUS_INFO_TGID_LABEL_NAME           = "tgid"
	PROC_PID_STATUS_INFO_REAL_UID_LABEL_NAME       = "real_uid"
	PROC_PID_STATUS_INFO_EFFECTIVE_UID_LABEL_NAME  = "effective_uid"
	PROC_PID_STATUS_INFO_SAVED_UID_LABEL_NAME      = "saved_uid"
	PROC_PID_STATUS_INFO_FILESYSTEM_UID_LABEL_NAME = "filesystem_uid"
	PROC_PID_STATUS_INFO_REAL_GID_LABEL_NAME       = "real_gid"
	PROC_PID_STATUS_INFO_EFFECTIVE_GID_LABEL_NAME  = "effective_gid"
	PROC_PID_STATUS_INFO_SAVED_GID_LABEL_NAME      = "saved_gid"
	PROC_PID_STATUS_INFO_FILESYSTEM_GID_LABEL_NAME = "filesystem_gid"

	PROC_PID_STATUS_VM_PEAK_METRIC_NAME = "proc_pid_status_vm_peak"

	PROC_PID_STATUS_VM_SIZE_METRIC_NAME = "proc_pid_status_vm_size"

	PROC_PID_STATUS_VM_LCK_METRIC_NAME = "proc_pid_status_vm_lck"

	PROC_PID_STATUS_VM_PIN_METRIC_NAME = "proc_pid_status_vm_pin"

	PROC_PID_STATUS_VM_HWM_METRIC_NAME = "proc_pid_status_vm_hwm"

	PROC_PID_STATUS_VM_RSS_METRIC_NAME = "proc_pid_status_vm_rss"

	PROC_PID_STATUS_VM_RSS_ANON_METRIC_NAME = "proc_pid_status_vm_rss_anon"

	PROC_PID_STATUS_VM_RSS_FILE_METRIC_NAME = "proc_pid_status_vm_rss_file"

	PROC_PID_STATUS_VM_RSS_SHMEM_METRIC_NAME = "proc_pid_status_vm_rss_shmem"

	PROC_PID_STATUS_VM_DATA_METRIC_NAME = "proc_pid_status_vm_data"

	PROC_PID_STATUS_VM_STK_METRIC_NAME = "proc_pid_status_vm_stk"

	PROC_PID_STATUS_VM_EXE_METRIC_NAME = "proc_pid_status_vm_exe"

	PROC_PID_STATUS_VM_LIB_METRIC_NAME = "proc_pid_status_vm_lib"

	PROC_PID_STATUS_VM_PTE_METRIC_NAME = "proc_pid_status_vm_pte"

	PROC_PID_STATUS_VM_PMD_METRIC_NAME = "proc_pid_status_vm_pmd"

	PROC_PID_STATUS_VM_SWAP_METRIC_NAME = "proc_pid_status_vm_swap"

	PROC_PID_STATUS_HUGETBL_PAGES_METRIC_NAME = "proc_pid_status_hugetbl_pages"

	PROC_PID_STATUS_VOLUNTARY_CTXT_SWITCHES_METRIC_NAME = "proc_pid_status_voluntary_ctxt_switches"

	PROC_PID_STATUS_NONVOLUNTARY_CTXT_SWITCHES_METRIC_NAME = "proc_pid_status_nonvoluntary_ctxt_switches"

	// procfs.ProcStatus.UIDs and .GIDs are [4]string for real, effective, saved
	// and filesystem accordingly:
	PROCFS_STATUS_UID_GID_REAL_INDEX       = 0
	PROCFS_STATUS_UID_GID_EFFECTIVE_INDEX  = 1
	PROCFS_STATUS_UID_GID_SAVED_INDEX      = 2
	PROCFS_STATUS_UID_GID_FILESYSTEM_INDEX = 3
)

var pidStatusInfoMetricFmt = fmt.Sprintf(
	`%s{%%s,%s="%%s",%s="%%d",%s="%%s",%s="%%s",%s="%%s",%s="%%s",%s="%%s",%s="%%s",%s="%%s",%s="%%s"}`,
	PROC_PID_STATUS_INFO_METRIC_NAME,
	PROC_PID_STATUS_INFO_NAME_LABEL_NAME,
	PROC_PID_STATUS_INFO_TGID_LABEL_NAME,
	PROC_PID_STATUS_INFO_REAL_UID_LABEL_NAME,
	PROC_PID_STATUS_INFO_EFFECTIVE_UID_LABEL_NAME,
	PROC_PID_STATUS_INFO_SAVED_UID_LABEL_NAME,
	PROC_PID_STATUS_INFO_FILESYSTEM_UID_LABEL_NAME,
	PROC_PID_STATUS_INFO_REAL_GID_LABEL_NAME,
	PROC_PID_STATUS_INFO_EFFECTIVE_GID_LABEL_NAME,
	PROC_PID_STATUS_INFO_SAVED_GID_LABEL_NAME,
	PROC_PID_STATUS_INFO_FILESYSTEM_GID_LABEL_NAME,
)

// Invoke for new/change in procStatus info:
func updateProcPidStatusInfoMetric(
	pmce *PidMetricsCacheEntry,
	procStatus *procfs.ProcStatus,
	promTs string,
	buf *bytes.Buffer,
	generatedCount *uint64,
) error {
	// Clear previous metric, if any:
	if pmce.ProcPidStatusInfoMetric != "" {
		buf.WriteString(pmce.ProcPidStatusInfoMetric + " 0 " + promTs + "\n")
		if generatedCount != nil {
			*generatedCount += 1
		}
	}
	// Update the metric:
	pmce.ProcPidStatusInfoMetric = fmt.Sprintf(
		pidStatusInfoMetricFmt,
		pmce.CommonLabels,
		procStatus.Name,
		procStatus.TGID,
		procStatus.UIDs[PROCFS_STATUS_UID_GID_REAL_INDEX],
		procStatus.UIDs[PROCFS_STATUS_UID_GID_EFFECTIVE_INDEX],
		procStatus.UIDs[PROCFS_STATUS_UID_GID_SAVED_INDEX],
		procStatus.UIDs[PROCFS_STATUS_UID_GID_FILESYSTEM_INDEX],
		procStatus.GIDs[PROCFS_STATUS_UID_GID_REAL_INDEX],
		procStatus.GIDs[PROCFS_STATUS_UID_GID_EFFECTIVE_INDEX],
		procStatus.GIDs[PROCFS_STATUS_UID_GID_SAVED_INDEX],
		procStatus.GIDs[PROCFS_STATUS_UID_GID_FILESYSTEM_INDEX],
	)
	return nil
}
