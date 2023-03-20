// Exercise proc_pid_metrics in a loop.

package main

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

const (
	MAX_FILE_READ_SIZE = 0x100000
)

type PidFileCacheKey struct {
	pid  int
	tid  int
	name string
}

type PidFileCache map[PidFileCacheKey][]byte

var ProcfsRoot = flag.String(
	"procfs-root",
	"/proc",
	"procfs root",
)
var ScanIntervalSeconds = flag.Float64(
	"scan-interval",
	1,
	"How often to scan, in seconds",
)
var ScanFactor = flag.Int(
	"scan-factor",
	1,
	"Repeat the scan N times for each interval",
)
var LogLevel = flag.String(
	"log-level",
	"info",
	"Set log level: error, info, debug, trace ...",
)

var PidFiles = []string{
	"cgroup",
	"cmdline",
	"io",
	"stat",
	"status",
}

func readPidFiles(
	procfsRoot string,
	pidList []pvmi.PidTidPair,
) (uint, uint64) {
	numFiles := uint(0)
	numBytes := uint64(0)
	buf := make([]byte, MAX_FILE_READ_SIZE)
	for _, pidTid := range pidList {
		var procDir string

		pid, tid := pidTid.GetPidTid()
		if tid == 0 {
			procDir = filepath.Join(procfsRoot, strconv.Itoa(pid))
		} else {
			procDir = filepath.Join(procfsRoot, strconv.Itoa(pid), "task", strconv.Itoa(tid))
		}

		for _, pidFile := range PidFiles {
			numFiles += 1
			filePath := filepath.Join(procDir, pidFile)
			file, err := os.Open(filePath)
			if err != nil {
				pvmi.Log.Warn(err)
				continue
			}
			n, err := file.Read(buf)
			if err != nil && err != io.EOF {
				pvmi.Log.Warnf("read(%s): %s", filePath, err)
				continue
			}
			numBytes += uint64(n)
		}
	}
	return numFiles, numBytes
}

func main() {
	flag.Parse()

	err := pvmi.SetLogLevelByName(*LogLevel)
	if err != nil {
		pvmi.Log.Fatal(err)
		return
	}

	debugEnabled := pvmi.Log.IsLevelEnabled(logrus.DebugLevel)

	scanInterval := time.Duration(*ScanIntervalSeconds * float64(time.Second))
	pidListCacheValidFor := scanInterval / 2
	if pidListCacheValidFor > time.Second {
		pidListCacheValidFor = time.Second
	}

	pvmi.Log.Infof(
		"ProcfsRoot: %s, ScanIntervalSeconds: %.03f, ScanFactor: %d",
		*ProcfsRoot,
		*ScanIntervalSeconds,
		*ScanFactor,
	)

	pidListNPart, pidListPart := 1, 0
	pidListCache := pvmi.NewPidListCache(
		pidListNPart,
		pidListCacheValidFor,
		*ProcfsRoot,
		pvmi.PID_LIST_CACHE_ALL_ENABLED_FLAG,
	)
	nextScan := time.Now()
	for {
		pause := time.Until(nextScan)
		if pause > 0 {
			time.Sleep(pause)
		}
		nextScan = nextScan.Add(scanInterval)

		start := time.Now()
		for i := 0; i < *ScanFactor; i++ {
			pidList := pidListCache.GetPids(pidListPart)
			start := time.Now()
			numFiles, numBytes := readPidFiles(*ProcfsRoot, pidList)
			duration := time.Since(start)
			if debugEnabled {
				pvmi.Log.Debugf("%d file(s), %d byte(s) read in %s", numFiles, numBytes, duration)
			}
		}
		if debugEnabled {
			duration := time.Since(start)
			pvmi.Log.Debug("Scan completed in ", duration)
		}
	}
}
