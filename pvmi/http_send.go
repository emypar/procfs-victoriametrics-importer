// HTTP sender
//
// Implement the function used by the compression pool for sending data to the
// import endpoints. Support multiple endpoints with health monitoring, see
// http_endpoint.go for details.

package pvmi

import (
	"context"
	"net/http"
	"sync"
	"time"
)

// The sender will use (*http.Client).Do; for test purposes define a mockable
// interface for it.
type HttpClientDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// The sender pool:
type HttpSenderPool struct {
	// Endpoints:
	endPoints *HttpEndpoints

	// HTTP client:
	httpTransport *http.Transport
	httpClient    HttpClientDoer

	// Pause between consecutive health checks:
	healthCheckPause time.Duration

	// How often to log the same health check error (i.e. log every Nth
	// occurrence). Note that every state change is logged. If <= 1 the log
	// every failure.
	logCheckFailureN int

	// The logistics for stopping running health check goroutines; needed during
	// testing:
	healthCheckCtx      context.Context
	healthCheckCancelFn context.CancelFunc
	healthCheckWg       *sync.WaitGroup

	// The new timer creation function, needed for testing:
	newTimerFn NewMockableTimerFn
}

func (pool *HttpSenderPool) GetImportUrl() string {
	return pool.endPoints.GetImportUrl()
}

func (pool *HttpSenderPool) Stop() {
	pool.healthCheckCancelFn()
	pool.healthCheckWg.Wait()
	Log.Info("Sender pool stopped")
}

func StartHealthCheck(ep *HttpEndpoint, pool *HttpSenderPool) error {
	importUrl, healthUrl := ep.importUrl, ep.healthUrl
	healthCheckPause := pool.healthCheckPause
	logCheckFailureN := pool.logCheckFailureN
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

	timer := newTimerFn(healthCheckPause, ep.importUrl)
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
			if prevStatusCode == response.StatusCode && prevErr == err {
				checkFailureN += 1
				if checkFailureN < logCheckFailureN {
					logCheckFailure = false
				}
			}
		}

	}()

	healthCheckWg.Add(1)
	return nil
}
