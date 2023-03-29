package pvmi

import (
	"net/http"
)

type MockableHttpClient interface {
	Do(*http.Request) (*http.Response, error)
	SetResponse(*http.Response, error)
}

type RealHttpClient struct {
	client *http.Client
}

func (d *RealHttpClient) Do(req *http.Request) (*http.Response, error) {
	return d.client.Do(req)
}

func (d *RealHttpClient) SetResponse(response *http.Response, err error) {
}

func NewRealHttpClient(client *http.Client) *RealHttpClient {
	return &RealHttpClient{client: client}
}

type MockHttpResponseFn func(req *http.Request) (*http.Response, error)

type MockHttpClient struct {
	responseFn MockHttpResponseFn
	request    *http.Request
	response   *http.Response
	err        error
}

func (m *MockHttpClient) Do(req *http.Request) (*http.Response, error) {
	m.request = req
	if m.responseFn != nil {
		return m.responseFn(req)
	} else {
		return m.response, m.err
	}
}

func (m *MockHttpClient) SetResponse(response *http.Response, err error) {
	m.response, m.err = response, err
}

func NewMockHttpClient(responseFn MockHttpResponseFn) *MockHttpClient {
	return &MockHttpClient{responseFn: responseFn}
}
