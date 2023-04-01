// Unit tests for http_send

package pvmi

import (
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

const (
	HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY = "*"
	HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS  = 50 * time.Millisecond
	HTTP_SEND_TEST_MAX_WAIT             = time.Second
	HTTP_SEND_TEST_MAX_NUM_POLLS        = int(
		HTTP_SEND_TEST_MAX_WAIT/HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS,
	) + 1
)

type MockHttpResponseErr struct {
	response *http.Response
	err      error
}

type HttpClientDoerMock struct {
	responseErrByMethodByUrl map[string]map[string]*MockHttpResponseErr
	requestBodyByMethodByUrl map[string]map[string][]byte
}

func NewHttpClientDoerMock() *HttpClientDoerMock {
	return &HttpClientDoerMock{
		responseErrByMethodByUrl: make(map[string]map[string]*MockHttpResponseErr),
		requestBodyByMethodByUrl: make(map[string]map[string][]byte),
	}
}

func (m *HttpClientDoerMock) setReqResponse(
	req *http.Request,
	response *http.Response,
	err error,
) {
	method, url := HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY, HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY
	if req != nil {
		method, url = req.Method, req.URL.String()
	}
	response_by_url := m.responseErrByMethodByUrl[method]
	if response_by_url == nil {
		response_by_url = make(map[string]*MockHttpResponseErr)
		m.responseErrByMethodByUrl[method] = response_by_url
	}
	response_by_url[url] = &MockHttpResponseErr{
		response,
		err,
	}
	if m.requestBodyByMethodByUrl[method] == nil {
		m.requestBodyByMethodByUrl[method] = make(map[string][]byte)
	}
}

func (m *HttpClientDoerMock) setGetResponse(
	url string,
	response *http.Response,
	err error,
) {
	method := http.MethodGet
	response_by_url := m.responseErrByMethodByUrl[method]
	if response_by_url == nil {
		response_by_url = make(map[string]*MockHttpResponseErr)
		m.responseErrByMethodByUrl[method] = response_by_url
	}
	response_by_url[url] = &MockHttpResponseErr{
		response,
		err,
	}
	if m.requestBodyByMethodByUrl[method] == nil {
		m.requestBodyByMethodByUrl[method] = make(map[string][]byte)
	}
}

func (m *HttpClientDoerMock) getResponseErr(req *http.Request) *MockHttpResponseErr {
	// Try: (method, url), (ANY, url), (method, ANY), (ANY, ANY):
	for _, url := range []string{req.URL.String(), HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY} {
		for _, method := range []string{req.Method, HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY} {
			response_by_url := m.responseErrByMethodByUrl[method]
			if response_by_url == nil {
				continue
			}
			responseErr := response_by_url[url]
			if responseErr != nil {
				return responseErr
			}
		}
	}
	return &MockHttpResponseErr{
		&http.Response{StatusCode: http.StatusNotFound},
		nil,
	}
}

func (m *HttpClientDoerMock) Do(req *http.Request) (*http.Response, error) {
	responseErr := m.getResponseErr(req)
	response, err := *responseErr.response, responseErr.err
	if response.Status == "" {
		response.Status = fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode))
	}
	if response.ProtoMajor == 0 {
		response.ProtoMajor = 1
	}
	if response.Proto == "" {
		response.Proto = fmt.Sprintf("HTTP%d/%d", response.ProtoMajor, response.ProtoMinor)
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		m.requestBodyByMethodByUrl[req.Method][req.URL.String()] = body
	}
	return &response, err
}

type MockableTimers struct {
	timers map[string]*testutils.MockTimer
}

func NewMockableTimers() *MockableTimers {
	return &MockableTimers{make(map[string]*testutils.MockTimer)}
}

func (mts *MockableTimers) NewMockTimer(d time.Duration, id string) MockableTimer {
	mockTimer := testutils.NewMockTimer()
	mts.timers[id] = mockTimer
	return MockableTimer(mockTimer)
}

func prepareHttpSenderPoolForTest(urlSpecList string) (
	pool *HttpSenderPool,
	importUrls []string,
	healthUrls []string,
	mockableTimers *MockableTimers,
	err error,
) {
	// Prepare config w/ mocks for timers and HTTP client:
	config := BuildHttpSendConfigFromArgs()
	config.urlSpecList = urlSpecList
	config.httpClient = NewHttpClientDoerMock()
	mockableTimers = NewMockableTimers()
	config.newTimerFn = mockableTimers.NewMockTimer
	pool, err = StartNewHttpSenderPool(config)
	if err != nil {
		return
	}

	// Extract import and health URLs in 2 parallel lists:
	importUrls = make([]string, 0)
	healthUrls = make([]string, 0)
	for importUrl, ep := range pool.endPoints.importUrlMap {
		importUrls = append(importUrls, importUrl)
		healthUrls = append(healthUrls, ep.healthUrl)
	}

	// Wait until all timers have registered; note that they use the import URL:
	allRegistered := false
	for pollN := 0; !allRegistered && pollN < HTTP_SEND_TEST_MAX_NUM_POLLS; pollN++ {
		if pollN > 0 {
			time.Sleep(HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS)
		}
		allRegistered = true
		for _, importUrl := range importUrls {
			_, ok := mockableTimers.timers[importUrl]
			if !ok {
				allRegistered = false
				break
			}
		}
	}
	if !allRegistered {
		pool.Stop()
		err = fmt.Errorf("Not all health checkers started after %s", HTTP_SEND_TEST_MAX_WAIT)
	}

	return
}

func testHttpEndpointsHealth(t *testing.T, urlSpecList string) {
	pool, importUrls, healthUrls, mockableTimers, err := prepareHttpSenderPoolForTest(urlSpecList)
	if err != nil {
		t.Fatal(err)
	}
	httpClientDoerMock := pool.httpClient.(*HttpClientDoerMock)

	defer func() {
		pool.Stop()
	}()

	// Each health checker is stopped in next check timer. Trigger them one by
	// one and expect the relevant import URL to be returned for use:
	for i, importUrl := range importUrls {
		healthUrl := healthUrls[i]
		httpClientDoerMock.setGetResponse(
			healthUrl,
			&http.Response{StatusCode: http.StatusOK},
			nil,
		)
		mockableTimers.timers[importUrl].Fire()
		// There should be i+1 healthy import URL's so in at most that many
		// calls to GetImportUrl the latest one should show up:
		nHealthyUrls := i + 1
		found := false
		for pollN := 0; !found && pollN < HTTP_SEND_TEST_MAX_NUM_POLLS; pollN++ {
			if pollN > 0 {
				time.Sleep(HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS)
			}
			for i := 0; i < nHealthyUrls; i++ {
				if pool.GetImportUrl() == importUrl {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("%s not found healthy after %s", importUrl, HTTP_SEND_TEST_MAX_WAIT)
		}
	}
}

func TestHttpSendEnpointsHealth(t *testing.T) {
	for _, urlSpecList := range []string{
		"http://1.1.1.1:8080",
		"http://1.1.1.1:8080,http://1.1.1.2:8080",
		"http://1.1.1.1:8080,http://1.1.1.2:8080,http://1.1.1.3:8080",
	} {
		t.Run(
			fmt.Sprintf("urlSpecList=%s", urlSpecList),
			func(t *testing.T) {
				testHttpEndpointsHealth(t, urlSpecList)
			},
		)
	}
}
