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

	pvmi.SetGlobalBufferPoolFromArgs()

	if *pvmi.HttpSendArgImportHttpEndpoints != "" {
		_, err = pvmi.StartNewHttpSenderPoolFromArgs()
		if err != nil {
			pvmi.Log.Fatal(err)
			return
		}
	}

}
