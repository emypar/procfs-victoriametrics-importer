// HTTP sender
//
// Implement the function used by the compression pool for sending data to the
// import endpoints. Support multiple endpoints with health monitoring, see
// http_endpoint.go for details.

package pvmi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var HttpSendLog = Log.WithField(
	LOGGER_COMPONENT_FIELD_NAME,
	"HttpSend",
)

var GlobalHttpSenderPool *HttpSenderPool

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

	// How long to wait for a successful send, including retries and wait for a
	// healthy URL:
	sendMaxWait time.Duration

	// How often to log the same health check error (i.e log only every Nth
	// consecutive occurrence):
	logNthCheckFailure int

	// Mocks during testing:
	newCancelableTimerFn NewCancelableTimerFn
	timeNowFn            TimeNowFn
}

func BuildHttpSendConfigFromArgs() *HttpSendConfig {
	maxConnsPerHost := *HttpSendMaxConnsPerHostArg
	if maxConnsPerHost <= 0 {
		maxConnsPerHost = GetNCompressorsFromArgs()
	}
	httpSendConfig := &HttpSendConfig{
		urlSpecList: *HttpSendImportHttpEndpointsArg,
		tcpConnectionTimeout: time.Duration(
			*HttpSendTcpConnectionTimeoutArg * float64(time.Second),
		),
		tcpKeepAlive: time.Duration(
			*HttpSendTcpKeepAliveArg * float64(time.Second),
		),
		httpDisableKeepAlives: *HttpSendDisableHttpKeepAliveArg,
		maxConnsPerHost:       maxConnsPerHost,
		idleConnTimeout: time.Duration(
			*HttpSendIdleConnectionTimeoutArg * float64(time.Second),
		),
		responseHeaderTimeout: time.Duration(
			*HttpSendResponseHeaderTimeoutArg * float64(time.Second),
		),
		healthCheckPause: time.Duration(
			*HttpSendHealthCheckPauseArg * float64(time.Second),
		),
		sendMaxWait: time.Duration(
			*HttpSendMaxWaitArg * float64(time.Second),
		),
		logNthCheckFailure: *HttpSendLogNthCheckFailureArg,
	}
	httpSendConfig.Log()
	return httpSendConfig
}

// The sender will use (*http.Client).Do; for test purposes define a mockable
// interface for it.
type HttpClientDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// The sender pool:
type HttpSenderPool struct {
	// Ingest the config:
	HttpSendConfig

	// Endpoints:
	*HttpEndpoints

	// The logistics for stopping running health check goroutines, needed during
	// testing:
	poolCtx       context.Context
	poolCancelFn  context.CancelFunc
	healthCheckWg *sync.WaitGroup
}

func NewHttpSenderPool(config *HttpSendConfig) (*HttpSenderPool, error) {
	if config == nil {
		config = BuildHttpSendConfigFromArgs()
	}
	eps, err := NewHttpEndpointsFromSpec(config.urlSpecList)
	if err != nil {
		return nil, err
	}

	poolCtx, poolCancelFn := context.WithCancel(context.Background())
	pool := &HttpSenderPool{
		HttpSendConfig: *config,
		HttpEndpoints:  eps,
		poolCtx:        poolCtx,
		poolCancelFn:   poolCancelFn,
		healthCheckWg:  &sync.WaitGroup{},
	}
	if pool.httpClient == nil {
		pool.httpClient = HttpClientDoer(
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
	if pool.newCancelableTimerFn == nil {
		pool.newCancelableTimerFn = NewCancelableTimer
	}
	if pool.timeNowFn == nil {
		pool.timeNowFn = time.Now
	}
	return pool, nil
}

func (pool *HttpSenderPool) Start() error {
	// Upon creation all endpoints are marked unhealthy; start the health checkers.s
	for _, ep := range pool.GetUnhealthyEndpoints() {
		err := pool.StartHealthCheck(ep)
		if err != nil {
			pool.Stop()
			return err
		}
	}
	return nil
}

func StartNewHttpSenderPool(config *HttpSendConfig) (*HttpSenderPool, error) {
	pool, err := NewHttpSenderPool(config)
	if err != nil {
		return nil, err
	}
	return pool, pool.Start()
}

func StartNewHttpSenderPoolFromArgs() (*HttpSenderPool, error) {
	return StartNewHttpSenderPool(nil)
}

func StartGlobalHttpSenderPoolFromArgs() error {
	pool, err := StartNewHttpSenderPoolFromArgs()
	if err != nil {
		return err
	}
	GlobalHttpSenderPool = pool
	return nil
}

func (pool *HttpSenderPool) Stop() {
	pool.poolCancelFn()
	pool.healthCheckWg.Wait()
	HttpSendLog.Info("Sender pool stopped")
}

func (pool *HttpSenderPool) DeclareImportUrlUnhealthy(importUrl string) error {
	ep := pool.MarkImportUrlUnhealthy(importUrl)
	if ep == nil {
		// Already marked:
		return nil
	}
	return pool.StartHealthCheck(ep)
}

func (pool *HttpSenderPool) StartHealthCheck(ep *HttpEndpoint) error {
	importUrl, healthUrl, httpClient := ep.importUrl, ep.healthUrl, pool.httpClient

	HttpSendLog.Infof("%s: Start health check using: %s", importUrl, healthUrl)
	method := http.MethodGet
	request, err := http.NewRequest(method, healthUrl, nil)
	if err != nil {
		return err
	}
	response, err := httpClient.Do(request)
	if response != nil && response.Body != nil {
		io.ReadAll(response.Body)
	}
	if err == nil && response.StatusCode == http.StatusOK {
		HttpSendLog.Infof("%s: Healthy", importUrl)
		pool.MarkHttpEndpointHealthy(ep)
		return nil
	}

	pool.healthCheckWg.Add(1)
	go func() {
		defer func() {
			HttpSendLog.Infof("%s: Stop health check", importUrl)
			pool.healthCheckWg.Done()
		}()

		healthCheckPause := pool.healthCheckPause
		logNthCheckFailure := pool.logNthCheckFailure
		checkFailureN, logCheckFailure := 0, true
		timer := pool.newCancelableTimerFn(pool.poolCtx, importUrl)
		timeNowFn := pool.timeNowFn
		for nextHealthCheck := timeNowFn().Add(healthCheckPause); ; nextHealthCheck = timeNowFn().Add(healthCheckPause) {
			if logCheckFailure {
				if err != nil {
					HttpSendLog.Warnf("%s unhealthy: %s", importUrl, err)
				} else {
					HttpSendLog.Warnf("%s unhealthy: %s %s: %s", importUrl, method, healthUrl, response.Status)
				}
				checkFailureN = 1
			}

			prevResponse, prevErr := response, err

			cancelled := timer.PauseUntil(nextHealthCheck)
			if cancelled {
				return
			}

			response, err = httpClient.Do(request)
			if response != nil && response.Body != nil {
				io.ReadAll(response.Body)
			}
			if err == nil && response.StatusCode == http.StatusOK {
				HttpSendLog.Infof("%s: Healthy", importUrl)
				pool.MarkHttpEndpointHealthy(ep)
				return
			}

			logCheckFailure = true
			if (prevErr != nil && err != nil && prevErr.Error() == err.Error()) ||
				(prevErr == nil && err == nil && prevResponse.StatusCode == response.StatusCode) {
				checkFailureN++
				if checkFailureN < logNthCheckFailure {
					logCheckFailure = false
				}
			}
		}

	}()

	return nil
}

func (pool *HttpSenderPool) Send(
	buf *bytes.Buffer,
	bufPool *BufferPool,
	contentEncoding string,
) error {
	var timer CancelablePauseTimer // Will be set JIT

	defer func() {
		if bufPool != nil {
			bufPool.ReturnBuffer(buf)
		}
	}()

	method := http.MethodPut
	contentEncodingHeader := []string{contentEncoding}
	contentLengthHeader := []string{strconv.Itoa(buf.Len())}
	bufId := "" // Will be set JIT
	body := bytes.NewReader(buf.Bytes())
	waitHealthyPause := pool.healthCheckPause + 20*time.Millisecond
	timeNowFn := pool.timeNowFn
	waitUntil := timeNowFn().Add(pool.sendMaxWait)
	for waitUntil.Sub(timeNowFn()) >= 0 {
		importUrl := pool.GetImportUrl(false)
		if importUrl == "" {
			if bufId == "" {
				HttpSendLog.Warnf("Waiting for a healthy import URL")
				bufId = fmt.Sprintf("Send(buf=%p)", buf)
				timer = pool.newCancelableTimerFn(pool.poolCtx, bufId)
			}
			cancelled := timer.PauseUntil(timeNowFn().Add(waitHealthyPause))
			if cancelled {
				return fmt.Errorf("Send cancelled")
			}
			continue
		}

		// Build the request; if that fails then there is no point trying again:
		body.Seek(0, io.SeekStart)
		request, err := http.NewRequest(
			method,
			importUrl,
			body,
		)
		if err != nil {
			return err
		}
		if contentEncoding != "" {
			request.Header["Content-Encoding"] = contentEncodingHeader
		}
		request.Header["Content-Length"] = contentLengthHeader

		response, err := pool.httpClient.Do(request)
		if response != nil && response.Body != nil {
			io.ReadAll(response.Body)
		}
		if err == nil && response.StatusCode == http.StatusOK {
			return nil
		}

		// Send failed, report the error and declare the endpoint as unhealthy:
		if err != nil {
			HttpSendLog.Warnf("%s %s: %s", method, importUrl, err)
		} else {
			HttpSendLog.Warnf("%s %s: %s", method, importUrl, response.Status)
		}
		err = pool.DeclareImportUrlUnhealthy(importUrl)
		if err != nil {
			return err
		}
	}
	return fmt.Errorf("send failed after %s", pool.sendMaxWait)
}

func (config *HttpSendConfig) Log() {
	HttpSendLog.Infof("HttpSendConfig: urlSpecList=%s", config.urlSpecList)
	HttpSendLog.Infof("HttpSendConfig: tcpConnectionTimeout=%s", config.tcpConnectionTimeout)
	HttpSendLog.Infof("HttpSendConfig: tcpKeepAlive=%s", config.tcpKeepAlive)
	HttpSendLog.Infof("HttpSendConfig: httpDisableKeepAlives=%v", config.httpDisableKeepAlives)
	HttpSendLog.Infof("HttpSendConfig: maxConnsPerHost=%d", config.maxConnsPerHost)
	HttpSendLog.Infof("HttpSendConfig: idleConnTimeout=%s", config.idleConnTimeout)
	HttpSendLog.Infof("HttpSendConfig: responseHeaderTimeout=%s", config.responseHeaderTimeout)
	HttpSendLog.Infof("HttpSendConfig: healthCheckPause=%s", config.healthCheckPause)
	HttpSendLog.Infof("HttpSendConfig: sendMaxWait=%s", config.sendMaxWait)
	HttpSendLog.Infof("HttpSendConfig: logNthCheckFailure=%d", config.logNthCheckFailure)
}
