package main

import "github.com/eparparita/procfs-victoriametrics-importer/pvmi"

func main() {
	pvmi.Log.Infof("Using clktckSec=%.06f", pvmi.ClktckSec)
	pvmi.Log.Infof("Using hostname=%#v", pvmi.Hostname)
}
