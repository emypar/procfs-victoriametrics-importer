// HTTP sender
//
// Implement the function used by the compression pool for sending data to the
// import endpoints. Support multiple endpoints with health monitoring, see
// http_endpoint.go for details.

package pvmi

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	DEFAULT_HTTP_SEND_TCP_CONNECTION_TIMEOUT  = 2.
	DEFAULT_HTTP_SEND_TCP_KEEP_ALIVE          = -1.
	DEFAULT_HTTP_SEND_IDLE_CONNECTION_TIMEOUT = 30.
	DEFAULT_HTTP_SEND_RESPONSE_HEADER_TIMEOUT = 10.
	DEFAULT_HTTP_SEND_HEALTH_CHECK_PAUSE      = 2.
	DEFAULT_HTTP_SEND_HEALTHY_IMPORT_MAX_WAIT = 10.
	DEFAULT_HTTP_SEND_LOG_NTH_CHECK_FAILURE   = 15
)

var HttpSendArgImportHttpEndpoints = flag.String(
	"import-http-endpoints",
	"",
	FormatFlagUsage(fmt.Sprintf(`
	Comma separated list of HTTP import endpoints. Each endpoint can be
	specified either as a  BASE_URL, in which case %s and %s are appended for
	import and health check accordingly, or explicitly as a
	(IMPORT_URL,HEALTH_CHECK_URL) pair. Mixing the 2 formats is supported,
	e.g.:
	BASE_URL,(IMPORT_URL,HEALTH_CHECK_URL),(IMPORT_URL,HEALTH_CHECK_URL),BASE_URL
	`, HTTP_ENDPOINT_IMPORT_URI, HTTP_ENDPOINT_HEALTH_URI)),
)

var HttpSendArgTcpConnectionTimeout = flag.Float64(
	"tcp-connection-timeout",
	DEFAULT_HTTP_SEND_TCP_CONNECTION_TIMEOUT,
	`TCP connection timeout, in seconds`,
)

var HttpSendArgTcpKeepAlive = flag.Float64(
	"tcp-keep-alive",
	DEFAULT_HTTP_SEND_TCP_KEEP_ALIVE,
	`TCP keep alive interval, in seconds; use -1 to disable`,
)

var HttpSendArgIdleConnectionTimeout = flag.Float64(
	"http-idle-connection-timeout",
	DEFAULT_HTTP_SEND_IDLE_CONNECTION_TIMEOUT,
	FormatFlagUsage(`
	The maximum amount of seconds an idle (keep-alive) connection will remain
	idle before closing itself; zero means no limit
	`),
)

var HttpSendArgDisableHttpKeepAlive = flag.Bool(
	"disable-http-keep-alive",
	false,
	`Disable HTTP keep alive`,
)

var HttpSendArgMaxConnsPerHost = flag.Int(
	"max-conns-per-host",
	-1,
	FormatFlagUsage(`
	The number of connections per endpoint, generally this should be the same
	as the number of compressors (assuming the worst case scenario with only
	one endpoint healthy); use -1 for the latter
	`),
)

var HttpSendArgResponseHeaderTimeout = flag.Float64(
	"http-response-header-timeout",
	DEFAULT_HTTP_SEND_RESPONSE_HEADER_TIMEOUT,
	FormatFlagUsage(`
	Response header timeout, if non-zero, specifies the amount of seconds to
	wait for a server's response headers after fully writing the request
	(including its body, if any). This time does not include the time to read
	the response body
	`),
)

var HttpSendArgHealthCheckPause = flag.Float64(
	"http-health-check-pause",
	DEFAULT_HTTP_SEND_HEALTH_CHECK_PAUSE,
	`Pause between consecutive health checks, in seconds`,
)

var HttpSendArgHealthyImportMaxWait = flag.Float64(
	"http-healthy-import-max-wait",
	DEFAULT_HTTP_SEND_HEALTHY_IMPORT_MAX_WAIT,
	`How long to wait for a healthy import, in seconds`,
)

var HttpSendArgLogNthCheckFailure = flag.Int(
	"http-health-log-nth-check-failure",
	DEFAULT_HTTP_SEND_LOG_NTH_CHECK_FAILURE,
	FormatFlagUsage(`
	How often to log the same health check error (i.e log only every Nth
	consecutive occurrence)
	`),
)

type HttpSendConfig struct {
	// The list of (IMPORT_URL,HEALTH_URL)|BASE_URL,...:
	urlSpecList string

	// The http client I/F to use, used for testing. If nil then the real
	// http.Client is used with a transport built based on transport params:
	httpClient HttpClientDoer

	// Transport params, ignored if a non nil client I/F is passed:
	// - TCP connection timeout:
	tcpConnectionTimeout time.Duration
	// - TCP keepalive, -1 to disable:
	tcpKeepAlive time.Duration

	// DisableKeepAlives, if true, disables HTTP keep-alives and
	// will only use the connection to the server for a single
	// HTTP request.
	//
	// This is unrelated to the similarly named TCP keep-alives.
	httpDisableKeepAlives bool

	// - the number of connections per endpoint, generally this should be the
	// same as the number of compressors (assuming the worst case scenario with
	// only one endpoint is healthy):
	maxConnsPerHost int
	// - the maximum amount of time an idle (keep-alive) connection will remain
	// idle before closing itself; zero means no limit:
	idleConnTimeout time.Duration
	// - response header timeout, if non-zero, specifies the amount of time to
	// wait for a server's response headers after fully writing the request
	// (including its body, if any). This time does not include the time to read
	// the response body.
	responseHeaderTimeout time.Duration

	// Pause between consecutive health checks:
	healthCheckPause time.Duration

	// Maximum number of attempts when waiting for a healthy import endpoint:
	getImportUrlMaxAttempts int

	// How often to log the same health check error (i.e log only every Nth
	// consecutive occurrence):
	logNthCheckFailure int

	// The timer creation function, used for testing. If nil then NewRealTimer
	// is used:
	newTimerFn NewMockableTimerFn
}

func BuildHttpSendConfigFromArgs() *HttpSendConfig {
	maxConnsPerHost := *HttpSendArgMaxConnsPerHost
	if maxConnsPerHost <= 0 {
		maxConnsPerHost = 1 // TO BE REVISED LATER!
	}
	return &HttpSendConfig{
		urlSpecList: *HttpSendArgImportHttpEndpoints,
		tcpConnectionTimeout: time.Duration(
			*HttpSendArgTcpConnectionTimeout * float64(time.Second),
		),
		tcpKeepAlive: time.Duration(
			*HttpSendArgTcpKeepAlive * float64(time.Second),
		),
		httpDisableKeepAlives: *HttpSendArgDisableHttpKeepAlive,
		maxConnsPerHost:       maxConnsPerHost,
		idleConnTimeout: time.Duration(
			*HttpSendArgIdleConnectionTimeout * float64(time.Second),
		),
		responseHeaderTimeout: time.Duration(
			*HttpSendArgResponseHeaderTimeout * float64(time.Second),
		),
		healthCheckPause: time.Duration(
			*HttpSendArgHealthCheckPause * float64(time.Second),
		),
		getImportUrlMaxAttempts: int(
			*HttpSendArgHealthyImportMaxWait / *HttpSendArgHealthCheckPause + 0.9,
		),
		logNthCheckFailure: *HttpSendArgLogNthCheckFailure,
	}
}

// The sender will use (*http.Client).Do; for test purposes define a mockable
// interface for it.
type HttpClientDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// The sender pool:
type HttpSenderPool struct {
	// Endpoints:
	endPoints *HttpEndpoints

	// HTTP client, can be a mock during testing:
	httpClient HttpClientDoer

	// Pause between consecutive health checks:
	healthCheckPause time.Duration

	// How many attempts to make when trying to get a healthy import URL:
	getImportUrlMaxAttempts int

	// How often to log the same health check error (i.e. log every Nth
	// occurrence). Note that every state change is logged. If <= 1 then log
	// every failure.
	logNthCheckFailure int

	// The logistics for stopping running health check goroutines, needed during
	// testing:
	healthCheckCtx      context.Context
	healthCheckCancelFn context.CancelFunc
	healthCheckWg       *sync.WaitGroup

	// The new timer creation function, can be a mock during testing:
	newTimerFn NewMockableTimerFn
}

func StartNewHttpSenderPool(config *HttpSendConfig) (*HttpSenderPool, error) {
	if config == nil {
		config = BuildHttpSendConfigFromArgs()
	}
	eps, err := NewHttpEndpointsFromSpec(config.urlSpecList)
	if err != nil {
		return nil, err
	}
	httpClient := config.httpClient
	if httpClient == nil {
		httpClient = HttpClientDoer(
			&http.Client{
				Transport: &http.Transport{
					DialContext: (&net.Dialer{
						Timeout:   config.tcpConnectionTimeout,
						KeepAlive: config.tcpKeepAlive,
					}).DialContext,
					DisableCompression:    true,
					DisableKeepAlives:     (config.httpDisableKeepAlives),
					MaxIdleConns:          0,
					MaxIdleConnsPerHost:   (config.maxConnsPerHost + 1) / 2,
					MaxConnsPerHost:       config.maxConnsPerHost,
					IdleConnTimeout:       config.idleConnTimeout,
					ResponseHeaderTimeout: config.responseHeaderTimeout,
				},
			},
		)
	}
	newTimerFn := config.newTimerFn
	if newTimerFn == nil {
		newTimerFn = NewRealTimer
	}

	healthCheckCtx, healthCheckCancelFn := context.WithCancel(context.Background())

	pool := &HttpSenderPool{
		endPoints:               eps,
		httpClient:              httpClient,
		healthCheckPause:        config.healthCheckPause,
		getImportUrlMaxAttempts: config.getImportUrlMaxAttempts,
		logNthCheckFailure:      config.logNthCheckFailure,
		healthCheckCtx:          healthCheckCtx,
		healthCheckCancelFn:     healthCheckCancelFn,
		healthCheckWg:           &sync.WaitGroup{},
		newTimerFn:              newTimerFn,
	}

	// Upon creation all endpoints are marked unhealthy; start the health checkers:
	for e := pool.endPoints.unhealthy.Front(); e != nil; e = e.Next() {
		ep := e.Value.(*HttpEndpoint)
		err = pool.StartHealthCheck(ep)
		if err != nil {
			pool.Stop()
			return nil, err
		}
	}

	return pool, nil
}

func (pool *HttpSenderPool) GetImportUrl() string {
	return pool.endPoints.GetImportUrl()
}

func (pool *HttpSenderPool) Stop() {
	pool.healthCheckCancelFn()
	pool.healthCheckWg.Wait()
	Log.Info("Sender pool stopped")
}

func (pool *HttpSenderPool) MarkImportUrlUnhealthy(importUrl string) error {
	ep := pool.endPoints.MarkImportUrlUnhealthy(importUrl)
	if ep == nil {
		// Already marked:
		return nil
	}
	return pool.StartHealthCheck(ep)
}

func (pool *HttpSenderPool) StartHealthCheck(ep *HttpEndpoint) error {
	importUrl, healthUrl := ep.importUrl, ep.healthUrl
	healthCheckPause := pool.healthCheckPause
	logNthCheckFailure := pool.logNthCheckFailure
	healthCheckCtx := pool.healthCheckCtx
	healthCheckWg := pool.healthCheckWg
	httpClient := pool.httpClient
	newTimerFn := pool.newTimerFn

	method := http.MethodGet
	request, err := http.NewRequest(method, healthUrl, nil)
	if err != nil {
		return err
	}

	Log.Infof("%s: Start health check", importUrl)
	checkFailureN, logCheckFailure, errMsg := 0, false, ""
	response, err := httpClient.Do(request)
	if err == nil && response.StatusCode == http.StatusOK {
		Log.Infof("%s: Healthy", importUrl)
		pool.endPoints.MarkHttpEndpointHealthy(ep)
		return nil
	}
	logCheckFailure = true

	timer := newTimerFn(healthCheckPause, importUrl)
	timerC := timer.GetChannel()
	go func() {
		defer func() {
			if !timer.Stop() {
				<-timerC
			}
			Log.Infof("%s: Stop health check", importUrl)
			healthCheckWg.Done()
		}()
		for {
			if logCheckFailure {
				if err != nil {
					errMsg = err.Error()
				} else {
					errMsg = response.Status
				}
				Log.Warnf("%s Unhealthy: %s %s: %s", importUrl, method, healthUrl, errMsg)
				checkFailureN = 1
			}

			prevStatusCode, prevErr := response.StatusCode, err

			select {
			case <-healthCheckCtx.Done():
				return
			case <-timerC:
			}

			timer.Reset(healthCheckPause)

			response, err := httpClient.Do(request)
			if err == nil && response.StatusCode == http.StatusOK {
				Log.Infof("%s: Healthy", importUrl)
				pool.endPoints.MarkHttpEndpointHealthy(ep)
				return
			}

			logCheckFailure = true
			if prevStatusCode == response.StatusCode &&
				(prevErr == nil && err == nil ||
					prevErr != nil && err != nil && prevErr.Error() != err.Error()) {
				checkFailureN += 1
				if checkFailureN < logNthCheckFailure {
					logCheckFailure = false
				}
			}
		}

	}()

	healthCheckWg.Add(1)
	return nil
}

func (pool *HttpSenderPool) Sender(
	buf *bytes.Buffer,
	bufPool *BufferPool,
	contentEncoding string,
) {
	defer func() {
		if bufPool != nil {
			bufPool.ReturnBuffer(buf)
		}
	}()

	method := http.MethodPut
	contentEncodingHeader := []string{contentEncoding}
	contentLengthHeader := []string{strconv.FormatInt(int64(buf.Len()), 10)}
	bufId := "" // Will be set JIT
	// Make as many attempts as there are endpoints, since one can transition to
	// unhealthy while being used:
	numEndpoints := pool.endPoints.numEndpoints
	// Wait slightly > health check interval for an URL to become healthy:
	waitHealthyPause := pool.healthCheckPause + 20*time.Millisecond
	getImportUrlMaxAttempts := pool.getImportUrlMaxAttempts
	for n := 0; n < numEndpoints; n++ {
		// Wait until a usable URL:
		importUrl := pool.GetImportUrl()
		for n := 0; importUrl == "" && n < getImportUrlMaxAttempts; n++ {
			if bufId == "" {
				bufId = fmt.Sprintf("Sender(%p)", buf)
			}
			// Note: use the mockable timer in case of testing:
			<-pool.newTimerFn(waitHealthyPause, bufId).GetChannel()
			importUrl = pool.GetImportUrl()
		}
		if importUrl == "" {
			Log.Warnf(
				"Could not get a healthy import URL after %d attempts, buffer discarded",
				getImportUrlMaxAttempts,
			)
		}

		// Build the request; if that fails then there is no point trying again:
		request, err := http.NewRequest(
			method,
			importUrl,
			buf,
		)
		if err != nil {
			Log.Errorf(
				"http.NewRequest(%s, %s, buf(%d bytes): %s",
				method,
				importUrl,
				buf.Len(),
				err,
			)
			return
		}
		if contentEncoding != "" {
			request.Header["Content-Encoding"] = contentEncodingHeader
		}
		request.Header["Content-Length"] = contentLengthHeader

		response, err := pool.httpClient.Do(request)
		if err == nil && response.StatusCode == http.StatusOK {
			return
		}

		// Send to this import URL failed, report the error and mark the
		// endpoint as unhealthy:
		if err != nil {
			Log.Warnf("%s %s: %s", method, importUrl, err)
		} else {
			Log.Warnf("%s %s: %s", method, importUrl, response.Status)
		}
		err = pool.MarkImportUrlUnhealthy(importUrl)
		if err != nil {
			Log.Warnf("Fail to mark %s unhealthy: %s", importUrl, err)
		}
	}
}
