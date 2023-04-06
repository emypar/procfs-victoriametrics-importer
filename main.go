package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

func main() {
	flag.Parse()

	err := pvmi.SetLoggerFromArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	pvmi.Log.Info("Start PVMI")

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
			pvmi.Log.Fatal(err)
			return
		}
		err = pvmi.StartGlobalCompressorPoolFromArgs(
			pvmi.GlobalMetricsWriteChannel,
			pvmi.GlobalBufPool,
			pvmi.GlobalHttpSenderPool.Send,
		)
		if err != nil {
			pvmi.Log.Fatal(err)
			return
		}
	}

	select {}
}
