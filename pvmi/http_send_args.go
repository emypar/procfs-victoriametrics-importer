// Command line args for http_send.

package pvmi

import (
	"flag"
	"fmt"
)

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
