// dummy sender for exercising the metrics generators (consume and discard)

package pvmi

import (
	"bytes"
	"flag"
	"os"
)

var DummySenderArg = flag.String(
	"dummy-sender",
	"",
	FormatFlagUsage(`
	Test flag, set to discard or stdout in which case the metrics are either
	ignored, useful to assess the resource utilization of the metrics
	generators, or displayed to stdout, useful to see examples of actual
	metrics without need of VictoriaMetrics infra.
	`),
)

func StartDummySenderFromArgs(wChan chan *bytes.Buffer, bufPool *BufferPool) {
	if wChan == nil {
		wChan = GlobalMetricsWriteChannel
	}
	display_metrics := *DummySenderArg == "stdout"
	if display_metrics {
		Log.Info("Start dummy sender, metrics will be displayed at stdout")
	} else {
		Log.Info("Start dummy sender, metrics will be discarded")
	}
	go func() {
		for buf := range wChan {
			if display_metrics {
				os.Stdout.Write(buf.Bytes())
			}
			if bufPool != nil {
				bufPool.ReturnBuffer(buf)
			}
		}
	}()
}
