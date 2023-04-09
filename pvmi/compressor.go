// gzip based compressor for metrics
//
// The compressor goroutines read from the metrics queue, to which metrics
// generators write, and compress metrics up to configurable size. Once a
// compressed batch was created, it is passed as an argument in the invocation
// of a sender function.
//
// The batch size cannot be assessed accurately because some of it is in the
// compression buffer, which is not exposed. However it can be estimated based
// on the number of bytes processed thus far, divided by the observed
// compression factor:
//
//      (compression factor = size(uncompressed) / size(compressed))
//
// The compression factor CF is updated at batch end, using exponential decay,
// alpha:
//
//      CF = (1 - alpha) * batchCF + alpha * CF, alpha = (0..1)

package pvmi

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"sync"
	"time"
)

const (
	DEFAULT_COMPRESSION_LEVEL               = gzip.DefaultCompression
	DEFAULT_COMPRESSED_BATCH_TARGET_SIZE    = 0x10000 // 64k
	DEFAULT_COMPRESSED_BATCH_FLUSH_INTERVAL = 5.      // seconds
	INITIAL_COMPRESSION_FACTOR              = 5.
	DEFAULT_COMPRESSION_FACTOR_ALPHA        = 0.5
	DEFAULT_NUM_COMPRESSOR_WORKERS          = -1
	MAX_NUM_COMPRESSOR_WORKERS              = 4
	// A compressed batch should have at least this size to be eligible for
	// compression ratio evaluation:
	COMPRESSED_BATCH_MIN_SIZE = 128
	// Special compressor ID for retrieving all stats:
	COMPRESSOR_ID_ALL = -1
)

var CompressorLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"Compressor",
)

type CompressorSenderFunction func(*bytes.Buffer, *BufferPool, string) error

type CompressorStats struct {
	m                 *sync.Mutex
	NReads            uint64
	NBytesRead        uint64
	NErrors           uint64
	NTimeoutFlushes   uint64
	NSends            uint64
	NBytesSent        uint64
	CompressionFactor float64
}

func NewCompressorStats() *CompressorStats {
	return &CompressorStats{
		m: &sync.Mutex{},
	}
}

func (cs *CompressorStats) GetCompressorStats() *CompressorStats {
	cs.m.Lock()
	csCopy := *cs // snapshot
	cs.m.Unlock()
	return &csCopy
}

type CompressorPoolContext struct {
	// Whether it was started or not:
	started bool
	// Compression level:
	compressionLevel int
	// Compressed batch target size; when the compressed data becomes ge than
	// the latter, the batch is sent out:
	batchTargetSize int
	// How long to wait before sending out a partially filled batch, to avoid
	// staleness. A timer is set with the value below when the batch starts and
	// if it fires before the target size is reached then the batch is sent out.
	flushInterval time.Duration
	// Compression factor exponential decay estimator parameter:
	alpha float64
	// The channel from which metrics are read:
	metricsWriteChan chan *bytes.Buffer
	// Buffer pool where to return the metrics buffers:
	bufPool *BufferPool
	// Sender function:
	senderFn CompressorSenderFunction
	// How many compressors to run:
	nCompressors int
	// Per compressor stats list:
	statsList []*CompressorStats
	// Cancel context used to stop all compressors:
	cancelCtx context.Context
	cancelFn  context.CancelFunc
	// Wait group to sync on exit:
	wg *sync.WaitGroup
}

var GlobalCompressorPoolContext *CompressorPoolContext

func StartNewCompressorPool(
	compressionLevel int,
	batchTargetSize int,
	flushInterval time.Duration,
	alpha float64,
	metricsWriteChan chan *bytes.Buffer,
	bufPool *BufferPool,
	senderFn CompressorSenderFunction,
	nCompressors int,
) (*CompressorPoolContext, error) {
	poolCtx, err := NewCompressorPoolContext(
		compressionLevel,
		batchTargetSize,
		flushInterval,
		alpha,
		metricsWriteChan,
		bufPool,
		senderFn,
		nCompressors,
	)
	if err != nil {
		return nil, err
	}
	err = poolCtx.Start()
	if err != nil {
		return nil, err
	}
	return poolCtx, nil
}

func AdjustNCompressors(nCompressors int) int {
	if nCompressors < 1 {
		nCompressors = CountAvailableCPUs()
		if nCompressors > MAX_NUM_COMPRESSOR_WORKERS {
			nCompressors = MAX_NUM_COMPRESSOR_WORKERS
		}
	}
	return nCompressors
}

func GetNCompressorsFromArgs() int {
	return AdjustNCompressors(*CompressorNumCompressorsArg)
}

func NewCompressorPoolContext(
	compressionLevel int,
	batchTargetSize int,
	flushInterval time.Duration,
	alpha float64,
	metricsWriteChan chan *bytes.Buffer,
	bufPool *BufferPool,
	senderFn CompressorSenderFunction,
	nCompressors int,
) (*CompressorPoolContext, error) {
	// Verify compression level:
	_, err := gzip.NewWriterLevel(gzip.NewWriter(nil), compressionLevel)
	if err != nil {
		return nil, err
	}
	nCompressors = AdjustNCompressors(nCompressors)
	poolCtx := &CompressorPoolContext{
		compressionLevel: compressionLevel,
		batchTargetSize:  batchTargetSize,
		flushInterval:    flushInterval,
		alpha:            alpha,
		metricsWriteChan: metricsWriteChan,
		bufPool:          bufPool,
		senderFn:         senderFn,
		nCompressors:     nCompressors,
		statsList:        make([]*CompressorStats, nCompressors),
		wg:               &sync.WaitGroup{},
	}
	poolCtx.cancelCtx, poolCtx.cancelFn = context.WithCancel(context.Background())
	return poolCtx, nil
}

func (poolCtx *CompressorPoolContext) SetMetricsWriteChan(metricsWriteChan chan *bytes.Buffer) error {
	if poolCtx.started {
		return fmt.Errorf("cannot set metricsWriteChan on running compressor pool")
	}
	poolCtx.metricsWriteChan = metricsWriteChan
	return nil
}

func (poolCtx *CompressorPoolContext) SetBufPool(bufPool *BufferPool) error {
	if poolCtx.started {
		return fmt.Errorf("cannot set bufPool on running compressor pool")
	}
	poolCtx.bufPool = bufPool
	return nil
}

func (poolCtx *CompressorPoolContext) SetSenderFn(senderFn CompressorSenderFunction) error {
	if poolCtx.started {
		return fmt.Errorf("cannot set senderFn on running compressor pool")
	}
	poolCtx.senderFn = senderFn
	return nil
}

func NewCompressorPoolContextFromArgs() (*CompressorPoolContext, error) {
	poolCtx, err := NewCompressorPoolContext(
		*CompressorCompressionLevelArg,
		*CompressorBatchTargetSizeArg,
		time.Duration(*CompressorArgBatchFlushInterval*float64(time.Second)),
		*CompressorExponentialDecayAlphaArg,
		nil,
		nil,
		nil,
		*CompressorNumCompressorsArg,
	)
	if err != nil {
		return nil, err
	}
	poolCtx.Log()
	return poolCtx, nil
}

func (poolCtx *CompressorPoolContext) Start() error {
	if poolCtx.started {
		return nil
	}
	for i := 0; i < poolCtx.nCompressors; i++ {
		poolCtx.statsList[i] = NewCompressorStats()
		// Note: the only reason for failure would be an illegal compression
		// level and this will be signaled by the 1st compressor. So in practice
		// this either fails right away or it succeeds for all.
		err := startCompressor(i, poolCtx)
		if err != nil {
			return err
		}
	}
	poolCtx.started = true
	return nil
}

func (poolCtx *CompressorPoolContext) Log() {
	CompressorLog.Infof("Compressor: compressionLevel=%d", poolCtx.compressionLevel)
	CompressorLog.Infof("Compressor: batchTargetSize=%d", poolCtx.batchTargetSize)
	CompressorLog.Infof("Compressor: flushInterval=%s", poolCtx.flushInterval)
	CompressorLog.Infof("Compressor: alpha=%f", poolCtx.alpha)
	CompressorLog.Infof("Compressor: nCompressors=%d", poolCtx.nCompressors)
}

func StartNewCompressorPoolFromArgs(
	metricsWriteChan chan *bytes.Buffer,
	bufPool *BufferPool,
	senderFn CompressorSenderFunction,
) (*CompressorPoolContext, error) {
	poolCtx, err := NewCompressorPoolContextFromArgs()
	if err != nil {
		return nil, err
	}
	err = poolCtx.SetMetricsWriteChan(metricsWriteChan)
	if err != nil {
		return nil, err
	}
	err = poolCtx.SetBufPool(bufPool)
	if err != nil {
		return nil, err
	}
	err = poolCtx.SetSenderFn(senderFn)
	if err != nil {
		return nil, err
	}
	err = poolCtx.Start()
	if err != nil {
		return nil, err
	}
	return poolCtx, nil
}

func StartGlobalCompressorPoolFromArgs(
	metricsWriteChan chan *bytes.Buffer,
	bufPool *BufferPool,
	senderFn CompressorSenderFunction,
) error {
	poolCtx, err := StartNewCompressorPoolFromArgs(
		metricsWriteChan,
		bufPool,
		senderFn,
	)
	if err != nil {
		return err
	}
	GlobalCompressorPoolContext = poolCtx
	return nil
}

func (poolCtx *CompressorPoolContext) Stop() {
	poolCtx.cancelFn()
	poolCtx.wg.Wait()
	CompressorLog.Info("Compressor pool stopped")
}

func (poolCtx *CompressorPoolContext) GetCompressorStats(id int) *CompressorStats {
	if id != COMPRESSOR_ID_ALL {
		return poolCtx.statsList[id].GetCompressorStats()
	}
	cumulativeStats := CompressorStats{}
	nCompression := 0
	for _, stats := range poolCtx.statsList {
		stats := stats.GetCompressorStats()
		cumulativeStats.NReads += stats.NReads
		cumulativeStats.NBytesRead += stats.NBytesRead
		cumulativeStats.NErrors += stats.NErrors
		cumulativeStats.NTimeoutFlushes += stats.NTimeoutFlushes
		cumulativeStats.NSends += stats.NSends
		cumulativeStats.NBytesSent += stats.NBytesSent
		if stats.NSends > 0 {
			cumulativeStats.CompressionFactor += stats.CompressionFactor
			nCompression += 1
		}
	}
	if nCompression > 0 {
		cumulativeStats.CompressionFactor /= float64(nCompression)
	}
	return &cumulativeStats
}

func startCompressor(id int, poolCtx *CompressorPoolContext) error {
	compressionLevel,
		batchTargetSize,
		flushInterval,
		alpha,
		metricsWriteChan,
		bufPool,
		senderFn,
		stats,
		cancelCtx,
		wg := poolCtx.compressionLevel,
		poolCtx.batchTargetSize,
		poolCtx.flushInterval,
		poolCtx.alpha,
		poolCtx.metricsWriteChan,
		poolCtx.bufPool,
		poolCtx.senderFn,
		poolCtx.statsList[id],
		poolCtx.cancelCtx,
		poolCtx.wg

	// Initialize a compressor writer; it cam use a dummy buffer since it will
	// be reset at the beginning of each compression batch.
	gzWriter, err := gzip.NewWriterLevel(gzip.NewWriter(nil), compressionLevel)
	if err != nil {
		return err
	}
	compressionFactor := INITIAL_COMPRESSION_FACTOR
	if compressionLevel == gzip.NoCompression {
		compressionFactor = 1.
	}

	// Initialize a stopped timer.
	flushTimer := time.NewTimer(time.Hour)
	if !flushTimer.Stop() {
		<-flushTimer.C
	}

	contentEncoding := "gzip"
	if wg != nil {
		wg.Add(1)
	}
	go func() {
		nBatchReads, nBatchBytesRead, doSend, timeoutFlush := 0, 0, false, false
		nBatchBytesLimit := int(float64(batchTargetSize) * compressionFactor)
		var gzBuf *bytes.Buffer
		for doLoop := true; doLoop; {
			select {
			case buf, isOpen := <-metricsWriteChan:
				if !isOpen {
					CompressorLog.Infof("Compressor# %d: Metrics write channel closed", id)
					doLoop = false
					doSend = nBatchReads > 0
					break
				}
				if nBatchReads == 0 {
					// First read of the batch, reset the compressor and set a timer:
					if bufPool != nil {
						gzBuf = bufPool.GetBuffer()
					} else {
						gzBuf = &bytes.Buffer{}
					}
					gzWriter.Reset(gzBuf)
					if flushInterval > 0 {
						flushTimer.Reset(flushInterval)
					}
				}
				nReadBytes := buf.Len()
				_, err := gzWriter.Write(buf.Bytes())
				if bufPool != nil {
					bufPool.ReturnBuffer(buf)
				} else {
					buf = nil // free its reference count
				}
				if err != nil {
					CompressorLog.Errorf("Compressor# %d: %s", id, err)
					// This should never happen, since the underlying writer is a
					// buffer, it should not run into errors. If it does, discard
					// the data thus far and reset the compressor.
					if flushInterval > 0 && !flushTimer.Stop() {
						<-flushTimer.C
					}
					if bufPool != nil {
						bufPool.ReturnBuffer(gzBuf)
					} else {
						gzBuf = nil // free its reference count
					}
					stats.m.Lock()
					stats.NErrors += 1
					stats.m.Unlock()
					nBatchReads, nBatchBytesRead, doSend, timeoutFlush = 0, 0, false, false
					break
				}
				nBatchReads += 1
				nBatchBytesRead += nReadBytes
				// Check for batch completion:
				if nBatchBytesRead >= nBatchBytesLimit {
					doSend = true
				}
			case <-cancelCtx.Done():
				CompressorLog.Infof("Compressor# %d: Pool shutting down", id)
				doLoop = false
				doSend = nBatchReads > 0
			case <-flushTimer.C:
				timeoutFlush = true
				doSend = true
			}
			if doSend {
				if flushInterval > 0 && !flushTimer.Stop() && !timeoutFlush {
					<-flushTimer.C
				}

				gzWriter.Close()
				nCompressedBytes := gzBuf.Len()
				if senderFn != nil {
					err := senderFn(gzBuf, bufPool, contentEncoding)
					if err != nil {
						CompressorLog.Warn(err)
					}
				} else if bufPool != nil {
					bufPool.ReturnBuffer(gzBuf)
				} else {
					gzBuf = nil
				}

				if compressionLevel != gzip.NoCompression &&
					nCompressedBytes >= COMPRESSED_BATCH_MIN_SIZE {
					compressionFactor = (1-alpha)*float64(nBatchBytesRead)/float64(nCompressedBytes) +
						alpha*compressionFactor
					nBatchBytesLimit = int(float64(batchTargetSize) * compressionFactor)
				}

				stats.m.Lock()
				stats.NReads += uint64(nBatchReads)
				stats.NBytesRead += uint64(nBatchBytesRead)
				stats.NSends += 1
				stats.NBytesSent += uint64(nCompressedBytes)
				stats.CompressionFactor = compressionFactor
				if timeoutFlush {
					stats.NTimeoutFlushes += 1
				}
				stats.m.Unlock()

				nBatchReads, nBatchBytesRead, doSend, timeoutFlush = 0, 0, false, false
			}
		}
		CompressorLog.Infof("Compressor# %d: Stopped", id)
		if wg != nil {
			wg.Done()
		}
	}()

	CompressorLog.Infof("Compressor# %d: Started", id)
	return nil
}
