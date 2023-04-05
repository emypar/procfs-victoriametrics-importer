package main

import (
	"flag"

	"github.com/eparparita/procfs-victoriametrics-importer/pvmi"
)

func main() {
	flag.Parse()

	pvmi.Log.Info("Start PVMI")
}
