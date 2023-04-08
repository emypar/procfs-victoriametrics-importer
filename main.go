package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

var MainLog = pvmi.Log.WithField(
	pvmi.COMPONENT_FIELD_NAME,
	"Main",
)

func main() {
	flag.Parse()

	err := pvmi.SetLoggerFromArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	MainLog.Info("Start PVMI")

	pvmi.SetGlobalMetricsWriteChannelFromArgs()
	pvmi.SetGlobalBufferPoolFromArgs()

	if *pvmi.DummySenderArg != "" {
		pvmi.StartDummySenderFromArgs(
			pvmi.GlobalMetricsWriteChannel,
			pvmi.GlobalBufPool,
		)
	} else {
		err = pvmi.StartGlobalHttpSenderPoolFromArgs()
		if err != nil {
			MainLog.Fatal(err)
			return
		}
		err = pvmi.StartGlobalCompressorPoolFromArgs(
			pvmi.GlobalMetricsWriteChannel,
			pvmi.GlobalBufPool,
			pvmi.GlobalHttpSenderPool.Send,
		)
		if err != nil {
			MainLog.Fatal(err)
			return
		}
	}

	select {}
}
