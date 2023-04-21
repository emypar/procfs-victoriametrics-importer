package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

var MainLog = pvmi.Log.WithField(
	pvmi.LOGGER_COMPONENT_FIELD_NAME,
	"Main",
)

func main() {
	flag.Parse()

	err := pvmi.SetLoggerFromArgs()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}

	// Set and verify common metrics pre-requisites:
	err = pvmi.SetCommonMetricsPreRequisitesFromArgs()
	if err != nil {
		MainLog.Fatal(err)
		return
	}

	MainLog.Info("Start PVMI")

	// Prepare the scheduler, compressor and sender framework:
	pvmi.SetGlobalBufferPoolFromArgs()
	pvmi.SetGlobalMetricsWriteChannelFromArgs()
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
	pvmi.StartGlobalSchedulerFromArgs()

	// Start the metrics generators:
	for _, startMetricsGeneratorFromArgsFn := range pvmi.GlobalStartGeneratorFromArgsList {
		err = startMetricsGeneratorFromArgsFn()
		if err != nil {
			MainLog.Fatal(err)
			return
		}
	}

	select {}
}
