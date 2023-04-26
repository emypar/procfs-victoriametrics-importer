// /proc/net/dev

package pvmi

import (
	"bytes"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

const (
	PROC_NET_DEV_RX_BYTES_METRIC_NAME      = "proc_net_dev_rx_bytes_total"
	PROC_NET_DEV_RX_PACKETS_METRIC_NAME    = "proc_net_dev_rx_packets_total"
	PROC_NET_DEV_RX_ERRORS_METRIC_NAME     = "proc_net_dev_rx_errors_total"
	PROC_NET_DEV_RX_DROPPED_METRIC_NAME    = "proc_net_dev_rx_dropped_total"
	PROC_NET_DEV_RX_FIFO_METRIC_NAME       = "proc_net_dev_rx_fifo_total"
	PROC_NET_DEV_RX_FRAME_METRIC_NAME      = "proc_net_dev_rx_frame_total"
	PROC_NET_DEV_RX_COMPRESSED_METRIC_NAME = "proc_net_dev_rx_compressed_total"
	PROC_NET_DEV_RX_MULTICAST_METRIC_NAME  = "proc_net_dev_rx_multicast_total"
	PROC_NET_DEV_TX_BYTES_METRIC_NAME      = "proc_net_dev_tx_bytes_total"
	PROC_NET_DEV_TX_PACKETS_METRIC_NAME    = "proc_net_dev_tx_packets_total"
	PROC_NET_DEV_TX_ERRORS_METRIC_NAME     = "proc_net_dev_tx_errors_total"
	PROC_NET_DEV_TX_DROPPED_METRIC_NAME    = "proc_net_dev_tx_dropped_total"
	PROC_NET_DEV_TX_FIFO_METRIC_NAME       = "proc_net_dev_tx_fifo_total"
	PROC_NET_DEV_TX_COLLISIONS_METRIC_NAME = "proc_net_dev_tx_collisions_total"
	PROC_NET_DEV_TX_CARRIER_METRIC_NAME    = "proc_net_dev_tx_carrier_total"
	PROC_NET_DEV_TX_COMPRESSED_METRIC_NAME = "proc_net_dev_tx_compressed_total"
	PROC_NET_DEV_DEVICE_LABEL_NAME         = "device"

	DEFAULT_PROC_NET_DEV_METRICS_SCAN_INTERVAL         = 1  // seconds
	DEFAULT_PROC_NET_DEV_METRICS_FULL_METRICS_INTERVAL = 15 // seconds
)

var ProcNetDevMetricsLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"ProcNetDevMetrics",
)

// The context for proc net dev metrics generation:
type ProcNetDevMetricsContext struct {
	// Interval, needed to qualify as a MetricsGenContext
	interval time.Duration
	// Full metrics factor, N. For delta strategy, metrics are generated only
	// for stats that changed from the previous run, however the full set is
	// generated at every Nth pass. Use  N = 1 for full metrics every time.
	fullMetricsFactor int64
	// The refresh cycle#, modulo fullMetricsFactor. Metrics are bundled
	// together in groups with group# also modulo fullMetricsFactor. Each time
	// group# == refresh cycle, all the metrics in the group are generated
	// regardless whether they changed or not from the previous scan:
	refreshCycleNum int64
	// The group# above is assigned at the time when a device is discovered, per
	// device, based on modulo fullMetricsFactor counter:
	newDevices          []string
	refreshGroupNum     map[string]int64
	nextRefreshGroupNum int64
	// procfs filesystem for procfs parsers:
	fs procfs.FS
	// The previous state, needed for delta strategy:
	prevNetDev procfs.NetDev
	// Timestamp of the prev scan:
	prevTs time.Time
	// Precomputed format for generating the metrics:
	counterMetricFmt string
	// The channel receiving the generated metrics:
	wChan chan *bytes.Buffer
	// The following are useful for testing, in lieu of mocks:
	hostname string
	job      string
	timeNow  TimeNowFn
	bufPool  *BufferPool
}

func (procNetDevMetricsCtx *ProcNetDevMetricsContext) GetInterval() time.Duration {
	return procNetDevMetricsCtx.interval
}

func NewProcNetDevMetricsContext(
	interval time.Duration,
	fullMetricsFactor int64,
	procfsRoot string,
	// needed for testing:
	hostname string,
	job string,
	timeNow TimeNowFn,
	wChan chan *bytes.Buffer,
	bufPool *BufferPool,
) (*ProcNetDevMetricsContext, error) {
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
	procNetDevMetricsCtx := &ProcNetDevMetricsContext{
		interval:            interval,
		fullMetricsFactor:   fullMetricsFactor,
		refreshCycleNum:     0,
		newDevices:          make([]string, 128)[:0],
		refreshGroupNum:     make(map[string]int64),
		nextRefreshGroupNum: 0,
		fs:                  fs,
		counterMetricFmt: fmt.Sprintf(
			`%%s{%s="%s",%s="%s",%s="%%s"} %%d %%s`+"\n",
			HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job, PROC_NET_DEV_DEVICE_LABEL_NAME,
		),
		wChan:    wChan,
		hostname: hostname,
		job:      job,
		timeNow:  timeNow,
		bufPool:  bufPool,
	}
	return procNetDevMetricsCtx, nil
}

func GenerateProcNetDevMetrics(mGenCtx MetricsGenContext) {
	procNetDevMetricsCtx := mGenCtx.(*ProcNetDevMetricsContext)
	netDev, err := procNetDevMetricsCtx.fs.NetDev()
	if err != nil {
		ProcNetDevMetricsLog.Warn(err)
		return
	}
	statsTs := procNetDevMetricsCtx.timeNow()
	promTs := strconv.FormatInt(statsTs.UnixMilli(), 10)

	fullMetricsFactor := procNetDevMetricsCtx.fullMetricsFactor
	deltaStrategy := fullMetricsFactor > 1

	prevNetDev := procNetDevMetricsCtx.prevNetDev
	newDevices := procNetDevMetricsCtx.newDevices[:0]
	refreshGroupNum := procNetDevMetricsCtx.refreshGroupNum
	refreshCycleNum := procNetDevMetricsCtx.refreshCycleNum

	counterMetricFmt := procNetDevMetricsCtx.counterMetricFmt
	bufPool := procNetDevMetricsCtx.bufPool
	buf := bufPool.GetBuffer()
	wChan := procNetDevMetricsCtx.wChan
	prevNetDevLine := procfs.NetDevLine{}
	for device, netDevLine := range netDev {
		fullMetrics := !deltaStrategy
		exists := false
		if deltaStrategy {
			prevNetDevLine, exists = prevNetDev[device]
			if exists {
				fullMetrics = refreshGroupNum[device] == refreshCycleNum
			} else {
				fullMetrics = true
				newDevices = append(newDevices, device)
			}
		}
		if fullMetrics || prevNetDevLine.RxBytes != netDevLine.RxBytes {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_BYTES_METRIC_NAME, device, netDevLine.RxBytes, promTs)
		}
		if fullMetrics || prevNetDevLine.RxPackets != netDevLine.RxPackets {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_PACKETS_METRIC_NAME, device, netDevLine.RxPackets, promTs)
		}
		if fullMetrics || prevNetDevLine.RxErrors != netDevLine.RxErrors {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_ERRORS_METRIC_NAME, device, netDevLine.RxErrors, promTs)
		}
		if fullMetrics || prevNetDevLine.RxDropped != netDevLine.RxDropped {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_DROPPED_METRIC_NAME, device, netDevLine.RxDropped, promTs)
		}
		if fullMetrics || prevNetDevLine.RxFIFO != netDevLine.RxFIFO {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_FIFO_METRIC_NAME, device, netDevLine.RxFIFO, promTs)
		}
		if fullMetrics || prevNetDevLine.RxFrame != netDevLine.RxFrame {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_FRAME_METRIC_NAME, device, netDevLine.RxFrame, promTs)
		}
		if fullMetrics || prevNetDevLine.RxCompressed != netDevLine.RxCompressed {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_COMPRESSED_METRIC_NAME, device, netDevLine.RxCompressed, promTs)
		}
		if fullMetrics || prevNetDevLine.RxMulticast != netDevLine.RxMulticast {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_RX_MULTICAST_METRIC_NAME, device, netDevLine.RxMulticast, promTs)
		}
		if fullMetrics || prevNetDevLine.TxBytes != netDevLine.TxBytes {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_BYTES_METRIC_NAME, device, netDevLine.TxBytes, promTs)
		}
		if fullMetrics || prevNetDevLine.TxPackets != netDevLine.TxPackets {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_PACKETS_METRIC_NAME, device, netDevLine.TxPackets, promTs)
		}
		if fullMetrics || prevNetDevLine.TxErrors != netDevLine.TxErrors {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_ERRORS_METRIC_NAME, device, netDevLine.TxErrors, promTs)
		}
		if fullMetrics || prevNetDevLine.TxDropped != netDevLine.TxDropped {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_DROPPED_METRIC_NAME, device, netDevLine.TxDropped, promTs)
		}
		if fullMetrics || prevNetDevLine.TxFIFO != netDevLine.TxFIFO {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_FIFO_METRIC_NAME, device, netDevLine.TxFIFO, promTs)
		}
		if fullMetrics || prevNetDevLine.TxCollisions != netDevLine.TxCollisions {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_COLLISIONS_METRIC_NAME, device, netDevLine.TxCollisions, promTs)
		}
		if fullMetrics || prevNetDevLine.TxCarrier != netDevLine.TxCarrier {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_CARRIER_METRIC_NAME, device, netDevLine.TxCarrier, promTs)
		}
		if fullMetrics || prevNetDevLine.TxCompressed != netDevLine.TxCompressed {
			fmt.Fprintf(buf, counterMetricFmt, PROC_NET_DEV_TX_COMPRESSED_METRIC_NAME, device, netDevLine.TxCompressed, promTs)
		}
	}
	if buf.Len() > 0 && wChan != nil {
		wChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}

	// Some housekeeping if delta strategy is in effect:
	if deltaStrategy {
		// Assign a refresh group# to each new device. To help with testing this
		// should be deterministic, but the new devices were discovered in hash
		// order (*), so the list will be sorted beforehand.
		// (*): that's still deterministic, technically, but only by reverse
		// engineering the hash function, i.e. by peeking into Go internals.
		if len(newDevices) > 0 {
			sort.Strings(newDevices)
			nextRefreshGroupNum := procNetDevMetricsCtx.nextRefreshGroupNum
			for _, device := range newDevices {
				refreshGroupNum[device] = nextRefreshGroupNum
				nextRefreshGroupNum += 1
				if nextRefreshGroupNum >= fullMetricsFactor {
					nextRefreshGroupNum = 0
				}
			}
			procNetDevMetricsCtx.nextRefreshGroupNum = nextRefreshGroupNum
			procNetDevMetricsCtx.newDevices = newDevices
		}

		// Check for devices that were removed; this may have occurred only if new
		// devices were discovered or the number of devices has changed:
		if len(newDevices) > 0 || prevNetDev != nil && len(prevNetDev) != len(netDev) {
			for device := range refreshGroupNum {
				_, exists := netDev[device]
				if !exists {
					delete(refreshGroupNum, device)
				}
			}
			procNetDevMetricsCtx.refreshGroupNum = refreshGroupNum
		}

		// Move to the next refresh cycle:
		refreshCycleNum += 1
		if refreshCycleNum >= fullMetricsFactor {
			refreshCycleNum = 0
		}
		procNetDevMetricsCtx.refreshCycleNum = refreshCycleNum

		// Finally update the prev scan info:
		procNetDevMetricsCtx.prevNetDev = netDev
		procNetDevMetricsCtx.prevTs = statsTs
	}
}

var ProcNetDevMetricsScanIntervalArg = flag.Float64(
	"proc-net-dev-metrics-scan-interval",
	DEFAULT_PROC_NET_DEV_METRICS_SCAN_INTERVAL,
	`proc_net_dev metrics interval in seconds, use 0 to disable.`,
)

var ProcNetDevMetricsFullMetricsIntervalArg = flag.Float64(
	"proc-net-dev-metrics-full-metrics-interval",
	DEFAULT_PROC_NET_DEV_METRICS_FULL_METRICS_INTERVAL,
	FormatFlagUsage(`
	How often to generate full metrics, in seconds; normally only the metrics
	whose value has changed from the previous scan are generated, but every
	so often the entire set is generated to prevent queries from having to go
	too much back in time to find the last value. Use 0 to generate full
	metrics at every scan.
	`),
)

func BuildProcNetDevMetricsCtxFromArgs() (*ProcNetDevMetricsContext, error) {
	if *ProcNetDevMetricsScanIntervalArg <= 0 {
		return nil, nil // i.e. disabled
	}
	interval := time.Duration(*ProcNetDevMetricsScanIntervalArg * float64(time.Second))
	fullMetricsInterval := time.Duration(*ProcNetDevMetricsFullMetricsIntervalArg * float64(time.Second))
	fullMetricsFactor := int64(fullMetricsInterval / interval)
	if fullMetricsFactor < 1 {
		fullMetricsFactor = 1
	}

	procNetDevMetricsCtx, err := NewProcNetDevMetricsContext(
		interval,
		fullMetricsFactor,
		GlobalProcfsRoot,
		// needed for testing:
		GlobalMetricsHostname,
		GlobalMetricsJob,
		// will be set to default values:
		nil, // timeNow TimeNowFn,
		nil, // wChan chan *bytes.Buffer,
		nil, // bufPool *BufferPool,
	)
	if err != nil {
		return nil, err
	}
	ProcNetDevMetricsLog.Infof("proc_net_dev metrics: interval=%s", interval)
	ProcNetDevMetricsLog.Infof("proc_net_dev metrics: procfsRoot=%s", GlobalProcfsRoot)
	ProcNetDevMetricsLog.Infof("proc_net_dev metrics: hostname=%s", GlobalMetricsHostname)
	ProcNetDevMetricsLog.Infof("proc_net_dev metrics: job=%s", GlobalMetricsJob)
	return procNetDevMetricsCtx, nil
}

var GlobalProcNetDevMetricsCtx *ProcNetDevMetricsContext

func StartProcNetDevMetricsFromArgs() error {
	procNetDevMetricsCtx, err := BuildProcNetDevMetricsCtxFromArgs()
	if err != nil {
		return err
	}
	if procNetDevMetricsCtx == nil {
		ProcNetDevMetricsLog.Warn("proc_net_dev metrics collection disabled")
		return nil
	}

	GlobalProcNetDevMetricsCtx = procNetDevMetricsCtx
	GlobalSchedulerContext.Add(GenerateProcNetDevMetrics, MetricsGenContext(procNetDevMetricsCtx))
	return nil
}

func init() {
	RegisterStartGeneratorFromArgs(StartProcNetDevMetricsFromArgs)
}
