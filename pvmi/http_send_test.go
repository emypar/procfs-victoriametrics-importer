// Unit tests for http_send

package pvmi

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

const (
	TEST_HTTP_SEND_PAUSE_BETWEEN_WAIT_HEALTHY_POLLS = 50 * time.Millisecond
	TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT             = time.Second
	TEST_HTTP_SEND_WAIT_HEALTHY_URL_MAX_NUM_POLLS   = int(
		TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT/TEST_HTTP_SEND_PAUSE_BETWEEN_WAIT_HEALTHY_POLLS,
	) + 1
	TEST_HTTP_SEND_HEALTH_CHECK_PAUSE    = 2 * time.Second
	TEST_HTTP_SEND_MAX_WAIT              = 10 * time.Second
	TEST_HTTP_SEND_LOG_NTH_CHECK_FAILURE = 15
)

var testHttpSendUrlSpecList = []string{
	"http://1.1.1.0:8080",
	"http://1.1.1.0:8080,http://1.1.1.1:8080",
	"http://1.1.1.0:8080,http://1.1.1.1:8080,http://1.1.1.2:8080",
	"(http://1.1.1.0:8080/import,http://1.1.1.0:8080/health),http://1.1.1.1:8080,http://1.1.1.2:8080",
}

func prepareHttpSenderPoolForTest(
	urlSpecList string,
	// The list of indexes in urlSpecList that should be started as healthy. If
	// it has only one element == -1 then start them all as healthy.
	startHealthyIndexList []int,
) (
	pool *HttpSenderPool,
	cancelableTimerMocks *testutils.CancelableTimerMockPool,
	timeNowMock *testutils.TimeNowMock,
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

	// Prepare config w/ mocks for timers and HTTP client:
	cancelableTimerMocks = testutils.NewCancelableTimerMockPool()
	newCancelableTimerFn := func(parentCtx context.Context, id string) CancelablePauseTimer {
		return CancelablePauseTimer(cancelableTimerMocks.NewCancelableTimer(parentCtx, id))
	}
	timeNowMock = testutils.NewTimeNowMock()
	config := &HttpSendConfig{
		urlSpecList:          urlSpecList,
		httpClient:           testutils.NewHttpClientDoerMock(),
		healthCheckPause:     TEST_HTTP_SEND_HEALTH_CHECK_PAUSE,
		sendMaxWait:          TEST_HTTP_SEND_MAX_WAIT,
		logNthCheckFailure:   TEST_HTTP_SEND_LOG_NTH_CHECK_FAILURE,
		newCancelableTimerFn: newCancelableTimerFn,
		timeNowFn:            timeNowMock.Now,
	}
	pool, err = NewHttpSenderPool(config)
	if err != nil {
		return
	}

	// Prepare HTTP client mocks for the import URLs that should check healthy
	// from the start:
	expectedImportUrlSet := make(map[string]bool)
	httpClientDoerMock := pool.httpClient.(*testutils.HttpClientDoerMock)
	if len(startHealthyIndexList) == 1 && startHealthyIndexList[0] == -1 {
		for i := 0; i < len(importHealthUrlPairs); i++ {
			expectedImportUrlSet[importUrls[i]] = true
			httpClientDoerMock.SetGetResponse(
				healthUrls[i],
				&http.Response{StatusCode: http.StatusOK},
				nil,
			)
		}
	} else {
		for _, i := range startHealthyIndexList {
			expectedImportUrlSet[importUrls[i]] = true
			httpClientDoerMock.SetGetResponse(
				healthUrls[i],
				&http.Response{StatusCode: http.StatusOK},
				nil,
			)
		}
	}

	err = pool.Start()
	if err != nil {
		return
	}

	// Wait until all expected healthy import URLs appear in the healthy list:
	for pollN := 0; len(expectedImportUrlSet) > 0 && pollN < TEST_HTTP_SEND_WAIT_HEALTHY_URL_MAX_NUM_POLLS; pollN++ {
		if pollN > 0 {
			time.Sleep(TEST_HTTP_SEND_PAUSE_BETWEEN_WAIT_HEALTHY_POLLS)
		}
		for _, ep := range pool.GetHealthyEndpoints() {
			delete(expectedImportUrlSet, ep.importUrl)
		}
	}
	if len(expectedImportUrlSet) > 0 {
		expectedHealthyUrlList := make([]string, len(expectedImportUrlSet))
		i := 0
		for importUrl, _ := range expectedImportUrlSet {
			expectedHealthyUrlList[i] = importUrl
			i++
		}
		err = fmt.Errorf(
			"the following importUrl are not healthy after %s: %v",
			TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT,
			expectedHealthyUrlList,
		)
	}

	return
}

func testHttpSendPoolStart(t *testing.T, urlSpecList string, startHealthyIndexList []int) {
	pool, _, _, err := prepareHttpSenderPoolForTest(
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
	for _, urlSpecList := range testHttpSendUrlSpecList {
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
	pool, cancelableTimerMocks, _, err := prepareHttpSenderPoolForTest(
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

	httpClientDoerMock := pool.httpClient.(*testutils.HttpClientDoerMock)

	// Each health checker is stopped in next check timer. Trigger them one by
	// one and expect the relevant import URL to be marked healthy:
	unhealthyEndpoints := pool.GetUnhealthyEndpoints()

	for _, ep := range unhealthyEndpoints {
		t.Logf("Waiting for %s...", ep.importUrl)
		importUrl, healthUrl := ep.importUrl, ep.healthUrl
		httpClientDoerMock.SetGetResponse(
			healthUrl,
			&http.Response{StatusCode: http.StatusOK},
			nil,
		)
		err := cancelableTimerMocks.FireWithTimeout(pool.poolCtx, importUrl, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for pollN := 0; !found && pollN < TEST_HTTP_SEND_WAIT_HEALTHY_URL_MAX_NUM_POLLS; pollN++ {
			if pollN > 0 {
				time.Sleep(TEST_HTTP_SEND_PAUSE_BETWEEN_WAIT_HEALTHY_POLLS)
			}
			for _, ep := range pool.GetHealthyEndpoints() {
				if ep.importUrl == importUrl {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("%s not found healthy after %s", importUrl, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
		}
	}
}

func TestHttpSendEnpointsBecomingHealthy(t *testing.T) {
	for _, urlSpecList := range testHttpSendUrlSpecList {
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

	pool, _, _, err := prepareHttpSenderPoolForTest(
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

	httpClientDoerMock := pool.httpClient.(*testutils.HttpClientDoerMock)
	for i, ep := range healthyEndpoints {
		if doFailover && i < len(healthyEndpoints)-1 {
			// Alternate the failure HTTP != OK v. error:
			if i&1 == 0 {
				response := &http.Response{StatusCode: http.StatusGone}
				httpClientDoerMock.SetGetResponse(ep.healthUrl, response, nil)
				httpClientDoerMock.SetPutResponse(ep.importUrl, response, nil)
			} else {
				httpClientDoerMock.SetGetResponse(ep.healthUrl, nil, fmt.Errorf("%s: dial error", ep.healthUrl))
				httpClientDoerMock.SetPutResponse(ep.importUrl, nil, fmt.Errorf("%s: dial error", ep.importUrl))
			}
		} else {
			response := &http.Response{StatusCode: http.StatusOK}
			httpClientDoerMock.SetGetResponse(ep.healthUrl, response, nil)
			httpClientDoerMock.SetPutResponse(ep.importUrl, response, nil)
		}
	}

	testDataMap := make(map[string]bool)
	for i := 0; i < 2*pool.NumEndpoints(); i++ {
		testDataMap[fmt.Sprintf("Test data# %d", i)] = true
	}

	for testData, _ := range testDataMap {
		buf := &bytes.Buffer{}
		buf.WriteString(testData)
		err := pool.Send(buf, nil, "")
		if err != nil {
			t.Fatal(err)
		}
	}

	requestBodyMap := httpClientDoerMock.RequestBodyByMethodByUrl[http.MethodPut]
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
	for _, urlSpecList := range testHttpSendUrlSpecList {
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

func testHttpSendNoImportUrl(
	t *testing.T,
	urlSpecList string,
	recoveryExpected bool,
) {
	pool, cancelableTimerMocks, timeNowMock, err := prepareHttpSenderPoolForTest(
		urlSpecList, nil,
	)

	defer func() {
		if pool != nil {
			pool.Stop()
		}
	}()

	if err != nil {
		t.Fatal(err)
	}

	buf := &bytes.Buffer{}
	buf.WriteString("Test no import list")
	bufId := fmt.Sprintf("Send(buf=%p)", buf)
	go func() {
		// Fire the timer keeping Send paused till next poll for a healthy URL:
		err := cancelableTimerMocks.FireWithTimeout(pool.poolCtx, bufId, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
		if err != nil {
			t.Log(err)
			return
		}
		// For the next attempt, either prepare a healthy URL (if recoveryExpected) or set time past the max wait:
		if recoveryExpected {
			for _, ep := range pool.importUrlMap {
				importUrl, healthUrl := ep.importUrl, ep.healthUrl
				httpClientDoerMock := pool.httpClient.(*testutils.HttpClientDoerMock)
				response := &http.Response{StatusCode: http.StatusOK}
				httpClientDoerMock.SetGetResponse(healthUrl, response, nil)
				httpClientDoerMock.SetPutResponse(importUrl, response, nil)
				err := cancelableTimerMocks.FireWithTimeout(pool.poolCtx, importUrl, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
				if err != nil {
					t.Log(err)
					return
				}
				found := false
				for pollN := 0; !found && pollN < TEST_HTTP_SEND_WAIT_HEALTHY_URL_MAX_NUM_POLLS; pollN++ {
					if pollN > 0 {
						time.Sleep(TEST_HTTP_SEND_PAUSE_BETWEEN_WAIT_HEALTHY_POLLS)
					}
					for _, ep := range pool.GetHealthyEndpoints() {
						if ep.importUrl == importUrl {
							found = true
							break
						}
					}
				}
				if !found {
					t.Logf("%s not found healthy after %s", importUrl, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
					return
				}
				break
			}
		} else {
			timeNowMock.Set(timeNowMock.Now().Add(2 * pool.sendMaxWait))
		}
		err = cancelableTimerMocks.FireWithTimeout(pool.poolCtx, bufId, TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
		if err != nil {
			t.Log(err)
			return
		}
	}()
	// Invoke send w/ a deadline:
	sendRet := make(chan error, 1)
	ctx, ctxCancelFn := context.WithTimeout(context.Background(), TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
	go func() {
		err := pool.Send(buf, nil, "")
		if err != nil {
			t.Log(err)
		}
		sendRet <- err
	}()
	select {
	case <-ctx.Done():
		t.Fatalf("Send timeout, it didn't complete after %s", TEST_HTTP_SEND_HEALTHY_URL_MAX_WAIT)
	case err = <-sendRet:
	}
	ctxCancelFn()
	if recoveryExpected {
		if err != nil {
			t.Fatal(err)
		}
	} else {
		if err == nil {
			t.Fatal("Unexpected Send success when no import URLs were available")
		}
	}

}

func TestHttpSendNoImportUrl(t *testing.T) {
	for _, urlSpecList := range testHttpSendUrlSpecList {
		for _, recoveryExpected := range []bool{false, true} {
			t.Run(
				fmt.Sprintf("urlSpecList=%s,recoveryExpected=%v", urlSpecList, recoveryExpected),
				func(t *testing.T) {
					testHttpSendNoImportUrl(t, urlSpecList, recoveryExpected)
				},
			)
		}
	}
}
