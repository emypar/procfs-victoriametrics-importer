// Unit tests for http_send

package pvmi

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

var TestHttpSendUrlSpecList = []string{
	"http://1.1.1.0:8080",
	"http://1.1.1.0:8080,http://1.1.1.1:8080",
	"http://1.1.1.0:8080,http://1.1.1.1:8080,http://1.1.1.2:8080",
	"(http://1.1.1.0:8080/import,http://1.1.1.0:8080/health),http://1.1.1.1:8080,http://1.1.1.2:8080",
}

type MockHttpResponseErr struct {
	response *http.Response
	err      error
}

type HttpClientDoerMock struct {
	responseErrByMethodByUrl map[string]map[string]*MockHttpResponseErr
	requestBodyByMethodByUrl map[string]map[string][][]byte
}

func NewHttpClientDoerMock() *HttpClientDoerMock {
	return &HttpClientDoerMock{
		responseErrByMethodByUrl: make(map[string]map[string]*MockHttpResponseErr),
		requestBodyByMethodByUrl: make(map[string]map[string][][]byte),
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
		m.requestBodyByMethodByUrl[method] = make(map[string][][]byte)
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
		m.requestBodyByMethodByUrl[method] = make(map[string][][]byte)
	}
}

func (m *HttpClientDoerMock) setPutResponse(
	url string,
	response *http.Response,
	err error,
) {
	method := http.MethodPut
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
		m.requestBodyByMethodByUrl[method] = make(map[string][][]byte)
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
	if responseErr.response.StatusCode == http.StatusOK &&
		responseErr.err == nil && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		m.requestBodyByMethodByUrl[req.Method][req.URL.String()] = append(
			m.requestBodyByMethodByUrl[req.Method][req.URL.String()],
			body,
		)
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

func prepareHttpSenderPoolForTest(
	urlSpecList string,
	// The list on indexes in urlSpecList that should be started as healthy. If
	// it has only one element == -1 then start them all as healthy.
	startHealthyIndexList []int,
) (
	pool *HttpSenderPool,
	mockableTimers *MockableTimers,
	err error,
) {
	defer func() {
		if err != nil && pool != nil {
			pool.Stop()
		}
	}()

	// Prepare the client mocks for the healthy import URLs. Set the
	// expectations for healthy/pending health checks lists:
	importHealthUrlPairs, parseErr := ParseEndpointSpec(urlSpecList)
	if parseErr != nil {
		err = parseErr
		return
	}
	importUrls := make([]string, 0)
	healthUrls := make([]string, 0)
	for _, importHealthUrlPair := range importHealthUrlPairs {
		importUrls = append(importUrls, importHealthUrlPair.importUrl)
		healthUrls = append(healthUrls, importHealthUrlPair.healthUrl)
	}

	healthyIndexSet := make(map[int]bool)
	unhealthyIndexSet := make(map[int]bool)
	if len(startHealthyIndexList) == 1 && startHealthyIndexList[0] == -1 {
		for i := 0; i < len(importHealthUrlPairs); i++ {
			healthyIndexSet[i] = true
		}
	} else {
		for _, i := range startHealthyIndexList {
			healthyIndexSet[i] = true
		}
		for i := 0; i < len(importHealthUrlPairs); i++ {
			if !healthyIndexSet[i] {
				unhealthyIndexSet[i] = true
			}
		}
	}

	// Prepare config w/ mocks for timers and HTTP client:
	config := BuildHttpSendConfigFromArgs()
	config.urlSpecList = urlSpecList
	config.httpClient = NewHttpClientDoerMock()
	mockableTimers = NewMockableTimers()
	config.newTimerFn = mockableTimers.NewMockTimer
	pool, err = NewHttpSenderPool(config)
	if err != nil {
		return
	}

	// Prepare HTTP client mocks for the import URLs that should check healthy
	// from the start:
	httpClientDoerMock := pool.httpClient.(*HttpClientDoerMock)
	for i, _ := range healthyIndexSet {
		httpClientDoerMock.setGetResponse(
			healthUrls[i],
			&http.Response{StatusCode: http.StatusOK},
			nil,
		)
	}

	err = pool.Start()
	if err != nil {
		return
	}

	// Wait until:
	//  - all expected healthy import URLs appear in the healthy list
	//  - all pending health check import URLs have timers registered
	allRegistered := false
	for pollN := 0; !allRegistered && pollN < HTTP_SEND_TEST_MAX_NUM_POLLS; pollN++ {
		if pollN > 0 {
			time.Sleep(HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS)
		}

		allRegistered = true

		foundHealthyImportUrls := make(map[string]bool)
		for _, ep := range pool.GetHealthyEndpoints() {
			foundHealthyImportUrls[ep.importUrl] = true
		}
		for i, _ := range healthyIndexSet {
			if !foundHealthyImportUrls[importUrls[i]] {
				allRegistered = false
				break
			}
		}
		if !allRegistered {
			continue
		}

		for i, _ := range unhealthyIndexSet {
			_, ok := mockableTimers.timers[importUrls[i]]
			if !ok {
				allRegistered = false
				break
			}
		}
	}
	if !allRegistered {
		err = fmt.Errorf("Not all health checkers started after %s", HTTP_SEND_TEST_MAX_WAIT)
	}

	return
}

func testHttpSendPoolStart(t *testing.T, urlSpecList string, startHealthyIndexList []int) {
	pool, _, err := prepareHttpSenderPoolForTest(
		urlSpecList, startHealthyIndexList,
	)
	defer func() {
		if pool != nil {
			pool.Stop()
		}
	}()

	if err != nil {
		t.Fatal(err)
	}
}

func makeIndexList(mask int) []int {
	indexList := make([]int, 0)
	for i := 0; mask > 0; i, mask = i+1, mask>>1 {
		if mask&1 > 0 {
			indexList = append(indexList, i)
		}
	}
	return indexList
}

func indexListToString(indexList []int) string {
	s := ""
	for k, i := range indexList {
		if k > 0 {
			s += ","
		}
		s += strconv.Itoa(i)
	}
	return s
}

func TestHttpSendPoolStart(t *testing.T) {
	for _, urlSpecList := range TestHttpSendUrlSpecList {
		importHealthUrlPairs, err := ParseEndpointSpec(urlSpecList)
		if err != nil {
			t.Log(err)
			continue
		}

		allIndex := []int{-1}

		for mask := 0; mask < 1<<len(importHealthUrlPairs); mask++ {
			startHealthyIndexList := makeIndexList(mask)
			t.Run(
				fmt.Sprintf(
					"urlSpecList=%s,startHealthyIndexList=%s",
					urlSpecList,
					indexListToString(startHealthyIndexList),
				),
				func(t *testing.T) {
					testHttpSendPoolStart(t, urlSpecList, startHealthyIndexList)
				},
			)
		}
		t.Run(
			fmt.Sprintf(
				"urlSpecList=%s,startHealthyIndexList=%s",
				urlSpecList,
				indexListToString(allIndex),
			),
			func(t *testing.T) {
				testHttpSendPoolStart(t, urlSpecList, allIndex)
			},
		)
	}
}

func testHttpEndpointsBecomingHealthy(t *testing.T, urlSpecList string, startHealthyIndexList []int) {
	pool, mockableTimers, err := prepareHttpSenderPoolForTest(
		urlSpecList, startHealthyIndexList,
	)

	defer func() {
		if pool != nil {
			pool.Stop()
		}
	}()

	if err != nil {
		t.Fatal(err)
	}

	httpClientDoerMock := pool.httpClient.(*HttpClientDoerMock)

	// Each health checker is stopped in next check timer. Trigger them one by
	// one and expect the relevant import URL to be marked healthy:
	unhealthyEndpoints := pool.GetUnhealthyEndpoints()

	for _, ep := range unhealthyEndpoints {
		importUrl, healthUrl := ep.importUrl, ep.healthUrl
		httpClientDoerMock.setGetResponse(
			healthUrl,
			&http.Response{StatusCode: http.StatusOK},
			nil,
		)
		mockableTimers.timers[importUrl].Fire()

		found := false
		for pollN := 0; !found && pollN < HTTP_SEND_TEST_MAX_NUM_POLLS; pollN++ {
			if pollN > 0 {
				time.Sleep(HTTP_SEND_TEST_PAUSE_BETWEEN_POLLS)
			}
			for _, ep := range pool.GetUnhealthyEndpoints() {
				if ep.importUrl == importUrl {
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

func TestHttpSendEnpointsBecomingHealthy(t *testing.T) {
	for _, urlSpecList := range TestHttpSendUrlSpecList {
		importHealthUrlPairs, err := ParseEndpointSpec(urlSpecList)
		if err != nil {
			t.Log(err)
			continue
		}

		for mask := 0; mask < 1<<len(importHealthUrlPairs); mask++ {
			startHealthyIndexList := makeIndexList(mask)
			t.Run(
				fmt.Sprintf(
					"urlSpecList=%s,startHealthyIndexList=%s",
					urlSpecList,
					indexListToString(startHealthyIndexList),
				),
				func(t *testing.T) {
					testHttpEndpointsBecomingHealthy(t, urlSpecList, startHealthyIndexList)
				},
			)
		}
	}
}

func testHttpSendOK(
	t *testing.T,
	urlSpecList string,
	startHealthyIndexList []int,
	doFailover bool,
) {
	if len(startHealthyIndexList) == 0 {
		return
	}

	pool, _, err := prepareHttpSenderPoolForTest(
		urlSpecList, startHealthyIndexList,
	)

	defer func() {
		if pool != nil {
			pool.Stop()
		}
	}()

	if err != nil {
		t.Fatal(err)
	}

	healthyEndpoints := pool.GetHealthyEndpoints()
	if doFailover && len(healthyEndpoints) < 2 {
		return
	}

	httpClientDoerMock := pool.httpClient.(*HttpClientDoerMock)
	for i, ep := range healthyEndpoints {
		var response *http.Response
		if doFailover && i < len(healthyEndpoints)-1 {
			response = &http.Response{StatusCode: http.StatusGone}
		} else {
			response = &http.Response{StatusCode: http.StatusOK}
		}
		httpClientDoerMock.setGetResponse(ep.healthUrl, response, nil)
		httpClientDoerMock.setPutResponse(ep.importUrl, response, nil)
	}

	testDataMap := make(map[string]bool)
	for i := 0; i < 2*pool.NumEndpoints(); i++ {
		testDataMap[fmt.Sprintf("Test data# %d", i)] = true
	}

	for testData, _ := range testDataMap {
		buf := &bytes.Buffer{}
		buf.WriteString(testData)
		pool.Send(buf, nil, "")
	}

	requestBodyMap := httpClientDoerMock.requestBodyByMethodByUrl[http.MethodPut]
	if requestBodyMap == nil {
		t.Fatalf("No requests were made")
	}

	for _, requestBodyList := range requestBodyMap {
		for _, requestBody := range requestBodyList {
			body := string(requestBody)
			if !testDataMap[body] {
				t.Errorf("Unexpected/duplicated request: %s", body)
			} else {
				delete(testDataMap, body)
			}
		}
	}

	for body, _ := range testDataMap {
		t.Errorf("missing request: %s", body)
	}
}

func TestHttpSendOK(t *testing.T) {
	for _, urlSpecList := range TestHttpSendUrlSpecList {
		importHealthUrlPairs, err := ParseEndpointSpec(urlSpecList)
		if err != nil {
			t.Log(err)
			continue
		}

		for mask := 1; mask < 1<<len(importHealthUrlPairs); mask++ {
			startHealthyIndexList := makeIndexList(mask)
			for _, doFailover := range []bool{false, true} {
				if doFailover && len(startHealthyIndexList) < 2 {
					continue
				}
				t.Run(
					fmt.Sprintf(
						"urlSpecList=%s,startHealthyIndexList=%s,doFailover=%v",
						urlSpecList,
						indexListToString(startHealthyIndexList),
						doFailover,
					),
					func(t *testing.T) {
						testHttpSendOK(t, urlSpecList, startHealthyIndexList, doFailover)
					},
				)
			}
		}
	}
}
