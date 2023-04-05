// Tests for compressor functions

package pvmi

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/gookit/goutil/dump"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

type CompressorTestCase struct {
	compressionLevel int
	batchTargetSize  int
	flushInterval    time.Duration
	alpha            float64
	nCompressors     int
	waitForFlush     bool
}

// Create a sender function that un-compresses and collects all data into a
// buffer:
func makeTestCompressorSenderFn(collectBuf *bytes.Buffer) CompressorSenderFunction {
	m := &sync.Mutex{}
	return func(gzBuf *bytes.Buffer, bufPool *BufferPool, contentEncoding string) error {
		m.Lock()
		defer m.Unlock()
		if contentEncoding == "gzip" {
			gzReader, _ := gzip.NewReader(gzBuf)
			collectBuf.ReadFrom(gzReader)
			gzReader.Close()
		} else {
			collectBuf.Write(gzBuf.Bytes())
		}
		if bufPool != nil {
			bufPool.ReturnBuffer(gzBuf)
		}
		return nil
	}
}

func testCompressor(
	t *testing.T,
	tc *CompressorTestCase,
	metrics []string,
) {
	collectBuf := &bytes.Buffer{}
	senderFn := makeTestCompressorSenderFn(collectBuf)
	bufPool := NewBufferPool(DEFAULT_BUF_POOL_MAX_SIZE)
	metricsWriteChan := make(chan *bytes.Buffer, 4*tc.nCompressors)

	t.Log("Create compressor pool")
	poolCtx, err := StartNewCompressorPool(
		tc.compressionLevel,
		tc.batchTargetSize,
		tc.flushInterval,
		tc.alpha,
		metricsWriteChan,
		bufPool,
		senderFn,
		tc.nCompressors,
	)
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		t.Log("Stop compressor pool")
		poolCtx.Stop()
	}()

	t.Log("Write metrics")
	buf := bufPool.GetBuffer()
	for _, metric := range metrics {
		buf.WriteString(metric)
		buf.WriteString("\n")
		if buf.Len() >= BUF_MAX_SIZE {
			metricsWriteChan <- buf
			buf = bufPool.GetBuffer()
		}
	}
	if buf.Len() >= 0 {
		metricsWriteChan <- buf
	} else {
		bufPool.ReturnBuffer(buf)
	}
	t.Log("All metrics written")

	if tc.waitForFlush {
		pause := tc.flushInterval + time.Second
		t.Logf("Wait %s (> %s flushInterval) for flush to occur...", pause, tc.flushInterval)
		time.Sleep(pause)
	} else {
		t.Log("Close metrics write channel")
		close(metricsWriteChan)

		t.Log("Wait for all compressors to complete")
		poolCtx.wg.Wait()
		t.Log("Compressors completed")
	}

	wantMetrics := testutils.DuplicateStrings(metrics, true)
	gotMetrics := testutils.BufToStrings(collectBuf, true)
	if diff := cmp.Diff(wantMetrics, gotMetrics); diff != "" {
		t.Errorf("Metrics mismatch (-want +got):\n%s", diff)
	}

	stats := poolCtx.GetCompressorStats(COMPRESSOR_ID_ALL)

	dumpBuf := &bytes.Buffer{}
	dumper := dump.NewDumper(dumpBuf, 3)
	dumper.NoColor = true
	dumper.ShowFlag = dump.Fnopos
	dumper.Dump(stats)
	t.Logf("Stats:\n%s\n", dumpBuf.String())
}

func TestCompressorFewMetrics(t *testing.T) {
	metrics := []string{
		`metric1{label="value"} 1 00000000000`,
		`metric2{label="value"} 2 00000000000`,
	}

	for _, tc := range []*CompressorTestCase{
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			5 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			1,
			false,
		},
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			1 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			1,
			true,
		},
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			5 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			4,
			false,
		},
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			1 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			4,
			true,
		},
		{
			gzip.NoCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			1 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			1,
			false,
		},
	} {
		t.Run(
			fmt.Sprintf(
				"NumComp=%d,TargetSz=%d,FlushSec=%.03f,Alpha=%.03f,Lvl=%d,Flush=%v",
				tc.nCompressors,
				tc.batchTargetSize,
				tc.flushInterval.Seconds(),
				tc.alpha,
				tc.compressionLevel,
				tc.waitForFlush,
			),
			func(t *testing.T) { testCompressor(t, tc, metrics) },
		)
	}
}

func TestCompressorPidMetrics(t *testing.T) {
	pmtcList := []PidMetricsTestCase{}
	err := testutils.LoadJsonFile(
		path.Join(TestdataTestCasesDir, PID_METRICS_TEST_CASES_FILE_NAME),
		&pmtcList,
	)
	if err != nil {
		t.Fatal(err)
	}
	metrics := make([]string, 0)
	for i := 0; i < 10; i++ {
		for _, pmtc := range pmtcList {
			metrics = append(metrics, pmtc.WantMetrics...)
		}
	}

	for _, tc := range []*CompressorTestCase{
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			5 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			1,
			false,
		},
		{
			gzip.DefaultCompression,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			5 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			4,
			false,
		},
		{
			gzip.BestSpeed,
			DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
			1 * time.Second,
			DEFAULT_COMPRESSION_FACTOR_ALPHA,
			4,
			true,
		},
	} {
		t.Run(
			fmt.Sprintf(
				"NumComp=%d,TargetSz=%d,FlushSec=%.03f,Alpha=%.03f,Lvl=%d,Flush=%v",
				tc.nCompressors,
				tc.batchTargetSize,
				tc.flushInterval.Seconds(),
				tc.alpha,
				tc.compressionLevel,
				tc.waitForFlush,
			),
			func(t *testing.T) { testCompressor(t, tc, metrics) },
		)
	}
}
