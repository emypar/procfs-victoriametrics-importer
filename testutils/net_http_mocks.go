package testutils

import (
	"fmt"
	"io"
	"net/http"
)

const (
	HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY = "*"
)

type MockHttpResponseErr struct {
	response *http.Response
	err      error
}

type HttpClientDoerMock struct {
	ResponseErrByMethodByUrl map[string]map[string]*MockHttpResponseErr
	RequestBodyByMethodByUrl map[string]map[string][][]byte
}

func NewHttpClientDoerMock() *HttpClientDoerMock {
	return &HttpClientDoerMock{
		ResponseErrByMethodByUrl: make(map[string]map[string]*MockHttpResponseErr),
		RequestBodyByMethodByUrl: make(map[string]map[string][][]byte),
	}
}

func (m *HttpClientDoerMock) SetReqResponse(
	req *http.Request,
	response *http.Response,
	err error,
) {
	method, url := HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY, HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY
	if req != nil {
		method, url = req.Method, req.URL.String()
	}
	response_by_url := m.ResponseErrByMethodByUrl[method]
	if response_by_url == nil {
		response_by_url = make(map[string]*MockHttpResponseErr)
		m.ResponseErrByMethodByUrl[method] = response_by_url
	}
	response_by_url[url] = &MockHttpResponseErr{
		response,
		err,
	}
	if m.RequestBodyByMethodByUrl[method] == nil {
		m.RequestBodyByMethodByUrl[method] = make(map[string][][]byte)
	}
}

func (m *HttpClientDoerMock) SetGetResponse(
	url string,
	response *http.Response,
	err error,
) {
	method := http.MethodGet
	response_by_url := m.ResponseErrByMethodByUrl[method]
	if response_by_url == nil {
		response_by_url = make(map[string]*MockHttpResponseErr)
		m.ResponseErrByMethodByUrl[method] = response_by_url
	}
	response_by_url[url] = &MockHttpResponseErr{
		response,
		err,
	}
	if m.RequestBodyByMethodByUrl[method] == nil {
		m.RequestBodyByMethodByUrl[method] = make(map[string][][]byte)
	}
}

func (m *HttpClientDoerMock) SetPutResponse(
	url string,
	response *http.Response,
	err error,
) {
	method := http.MethodPut
	response_by_url := m.ResponseErrByMethodByUrl[method]
	if response_by_url == nil {
		response_by_url = make(map[string]*MockHttpResponseErr)
		m.ResponseErrByMethodByUrl[method] = response_by_url
	}
	response_by_url[url] = &MockHttpResponseErr{
		response,
		err,
	}
	if m.RequestBodyByMethodByUrl[method] == nil {
		m.RequestBodyByMethodByUrl[method] = make(map[string][][]byte)
	}
}

func (m *HttpClientDoerMock) GetResponseErr(req *http.Request) *MockHttpResponseErr {
	// Try: (method, url), (ANY, url), (method, ANY), (ANY, ANY):
	for _, url := range []string{req.URL.String(), HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY} {
		for _, method := range []string{req.Method, HTTP_SEND_TEST_HTTP_CLIENT_MOCK_ANY} {
			response_by_url := m.ResponseErrByMethodByUrl[method]
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
	responseErr := m.GetResponseErr(req)
	response, err := responseErr.response, responseErr.err
	if response != nil {
		if response.Status == "" {
			response.Status = fmt.Sprintf("%d %s", response.StatusCode, http.StatusText(response.StatusCode))
		}
		if response.ProtoMajor == 0 {
			response.ProtoMajor = 1
		}
		if response.Proto == "" {
			response.Proto = fmt.Sprintf("HTTP%d/%d", response.ProtoMajor, response.ProtoMinor)
		}
		if response.StatusCode == http.StatusOK && err == nil && req.Body != nil {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			m.RequestBodyByMethodByUrl[req.Method][req.URL.String()] = append(
				m.RequestBodyByMethodByUrl[req.Method][req.URL.String()],
				body,
			)
		}
	}
	return response, err
}
