// Command line argument definitions:

package pvmi

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"strings"
)

const (
	DEFAULT_USAGE_WIDTH = 58
)

// http_send.go:
var HttpSendImportHttpEndpointsArg = flag.String(
	"http-send-import-endpoints",
	"",
	FormatFlagUsage(fmt.Sprintf(`
	Comma separated list of HTTP import endpoints. Each endpoint can be
	specified either as a  BASE_URL, in which case %#v and %#v are appended for
	import and health check accordingly, or explicitly as a
	(IMPORT_URL,HEALTH_CHECK_URL) pair. Mixing the 2 formats is supported,
	e.g.:
	BASE_URL,(IMPORT_URL,HEALTH_CHECK_URL),(IMPORT_URL,HEALTH_CHECK_URL),BASE_URL
	`, HTTP_ENDPOINT_IMPORT_URI, HTTP_ENDPOINT_HEALTH_URI)),
)

var HttpSendTcpConnectionTimeoutArg = flag.Float64(
	"http-send-tcp-connection-timeout",
	DEFAULT_HTTP_SEND_TCP_CONNECTION_TIMEOUT,
	`TCP connection timeout, in seconds`,
)

var HttpSendTcpKeepAliveArg = flag.Float64(
	"http-send-tcp-keep-alive",
	DEFAULT_HTTP_SEND_TCP_KEEP_ALIVE,
	`TCP keep alive interval, in seconds; use -1 to disable`,
)

var HttpSendIdleConnectionTimeoutArg = flag.Float64(
	"http-send-idle-connection-timeout",
	DEFAULT_HTTP_SEND_IDLE_CONNECTION_TIMEOUT,
	FormatFlagUsage(`
	The maximum amount of seconds an idle (keep-alive) connection will remain
	idle before closing itself; zero means no limit
	`),
)

var HttpSendDisableHttpKeepAliveArg = flag.Bool(
	"http-send-disable-keep-alive",
	false,
	`Disable HTTP keep alive`,
)

var HttpSendMaxConnsPerHostArg = flag.Int(
	"http-send-max-conns-per-host",
	-1,
	FormatFlagUsage(`
	The number of connections per endpoint, generally this should be the same
	as the number of compressors (assuming the worst case scenario with only
	one endpoint healthy); use -1 for the latter
	`),
)

var HttpSendResponseHeaderTimeoutArg = flag.Float64(
	"http-send-response-header-timeout",
	DEFAULT_HTTP_SEND_RESPONSE_HEADER_TIMEOUT,
	FormatFlagUsage(`
	Response header timeout, if non-zero, specifies the amount of seconds to
	wait for a server's response headers after fully writing the request
	(including its body, if any). This time does not include the time to read
	the response body
	`),
)

var HttpSendHealthCheckPauseArg = flag.Float64(
	"http-send-health-check-pause",
	DEFAULT_HTTP_SEND_HEALTH_CHECK_PAUSE,
	`Pause between consecutive health checks, in seconds`,
)

var HttpSendHealthyImportMaxWaitArg = flag.Float64(
	"http-send-healthy-import-max-wait",
	DEFAULT_HTTP_SEND_HEALTHY_IMPORT_MAX_WAIT,
	`How long to wait for a healthy import, in seconds`,
)

var HttpSendLogNthCheckFailureArg = flag.Int(
	"http-health-log-nth-check-failure",
	DEFAULT_HTTP_SEND_LOG_NTH_CHECK_FAILURE,
	FormatFlagUsage(`
	How often to log the same health check error (i.e log only every Nth
	consecutive occurrence).
	`),
)

// compressor.go:
var CompressorNumCompressorsArg = flag.Int(
	"compressor-num-workers",
	DEFAULT_NUM_COMPRESSOR_WORKERS,
	FormatFlagUsage(fmt.Sprintf(`
	The number of compressor workers. If -1, then the number will be
	calculated as min(available CPU#, %d).
	`, MAX_NUM_COMPRESSOR_WORKERS)),
)

var CompressorCompressionLevelArg = flag.Int(
	"compressor-compression-level",
	DEFAULT_COMPRESSION_LEVEL,
	FormatFlagUsage(fmt.Sprintf(`
	Compression level. Use %d for default compression and %d for no
	compression accordingly.
	`, gzip.DefaultCompression, gzip.NoCompression)),
)

var CompressorBatchTargetSizeArg = flag.Int(
	"compressor-batch-target-size",
	DEFAULT_COMPRESSED_BATCH_TARGET_SIZE,
	FormatFlagUsage(`
	Compress until the current batch reaches this size.
	`),
)

var CompressorArgBatchFlushInterval = flag.Float64(
	"compressor-batch-flush-interval",
	DEFAULT_COMPRESSED_BATCH_FLUSH_INTERVAL,
	FormatFlagUsage(`
	If the current compressed batch doesn't reach the target size in this many
	seconds since it was started, send it anyway to prevent stale data.
	`),
)

var CompressorExponentialDecayAlphaArg = flag.Float64(
	"compressor-exponential-decay-alpha",
	DEFAULT_COMPRESSION_FACTOR_ALPHA,
	FormatFlagUsage(`
	The estimated compression factor, needed for computing compressed batch
	size during compression, is revised after each batch using the following
	formula: CF = (1 - alpha) * batchCF + alpha * CF, alpha = (0..1).
	`),
)

// buffer_pool.go:
var BufPoolMaxSizeArg = flag.Int(
	"buffer-pool-max-size",
	DEFAULT_BUF_POOL_MAX_SIZE,
	FormatFlagUsage(fmt.Sprintf(`
	The application uses a general purpose recyclable bytes.Buffer pool with
	2 main methods: GetBuffer and ReturnBuffer. GetBuffer will return new
	buffers as needed, in an unbound fashion, whereas ReturnBuffer will
	discard buffers after this limit is reached. Use %d to make it unbound
	for return too.
	`, BUF_POOL_MAX_SIZE_UNBOUND)),
)

// logger.go:
var LoggerUseJsonArg = flag.Bool(
	"log-json-format",
	false,
	"Enable log in JSON format.",
)

var LoggerLevelArg = flag.String(
	"log-level",
	DEFAULT_LOG_LEVEL.String(),
	FormatFlagUsage(fmt.Sprintf(`
	Set log level, it should be one of the %s values. 
	`, GetLogLevelNames())),
)

// metrics_common.go:
var MetricsWriteChanSizeArg = flag.Int(
	"metrics-write-channel-size",
	DEFAULT_METRICS_WRITE_CHANNEL_SIZE,
	FormatFlagUsage(fmt.Sprintf(`
	The size of the metrics write channel (AKA Compressor Queue). Use %d to
	base it on buffer-pool-max-size arg, 90%% of that value. The value will
	be adjusted to be at least %d.
	`,
		METRICS_WRITE_CHANNEL_BASED_ON_BUF_POOL_SIZE,
		METRICS_WRITE_CHANNEL_MIN_SIZE,
	)),
)

// dummy_wchan.go:
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

// Format command flag usage for help message.
func FormatFlagUsageWidth(usage string, width int) string {
	buf := &bytes.Buffer{}
	lineLen := 0
	for _, word := range strings.Fields(strings.TrimSpace(usage)) {
		if lineLen == 0 {
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen = n
		} else {
			if lineLen+len(word)+1 > width {
				buf.WriteByte('\n')
				lineLen = 0
			} else {
				buf.WriteByte(' ')
				lineLen++
			}
			n, err := buf.WriteString(word)
			if err != nil {
				return usage
			}
			lineLen += n
		}
	}
	return buf.String()
}

func FormatFlagUsage(usage string) string {
	return FormatFlagUsageWidth(usage, DEFAULT_USAGE_WIDTH)
}
