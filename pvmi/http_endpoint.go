// The sender function invoked by compressor to deliver data to import
// endpoints.
//
// The importer may be configured w/ a list of http endpoints which will be
// stored internally into a pool.
//
// Each endpoint can be either healthy or unhealthy; the pool maintains a
// separate list for each condition. The members of the unhealthy list are
// checked periodically via a test request and when the latter succeeds the
// member is moved to the healthy list.
//
// Senders will request an endpoint and if they encounter an error while using
// it, they will report it to the pool where it will be placed into the unhealthy
// list. The sender will then request a new end point until either it succeeds
// with the request or there are no more healthy endpoints. If the latter, the
// sender will drop the data.

package pvmi

import (
	"container/list"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"
)

const (
	HTTP_ENDPOINT_HEALTH_URI = "/ready"
	HTTP_ENDPOINT_IMPORT_URI = "/api/v1/import/prometheus"
)

type HttpEndpoint struct {
	// The list it belongs to, healthy/unhealthy:
	list *list.List

	// The element in that list:
	listElement *list.Element

	// Import URL:
	importUrl string

	// Health check URL:
	healthUrl string
}

type HttpEndpoints struct {
	m            *sync.Mutex
	healthy      *list.List
	unhealthy    *list.List
	importUrlMap map[string]*HttpEndpoint
}

func NewHttpEndpoints() *HttpEndpoints {
	return &HttpEndpoints{
		m:            &sync.Mutex{},
		healthy:      list.New(),
		unhealthy:    list.New(),
		importUrlMap: map[string]*HttpEndpoint{},
	}
}

func (eps *HttpEndpoints) Counts() (int, int) {
	eps.m.Lock()
	defer eps.m.Unlock()

	return eps.healthy.Len(), eps.unhealthy.Len()
}

// New URLs are added by default to the unhealthy list:
func (eps *HttpEndpoints) AddHttpUrls(importUrl, healthUrl string) {
	eps.m.Lock()
	defer eps.m.Unlock()

	// Check if already added:
	if eps.importUrlMap[importUrl] != nil {
		return
	}
	// Add it:
	ep := &HttpEndpoint{
		list:      eps.unhealthy,
		importUrl: importUrl,
		healthUrl: healthUrl,
	}
	ep.listElement = ep.list.PushBack(ep)
	eps.importUrlMap[importUrl] = ep
}

func HttpEndpointUrlsFromBaseUrl(baseUrl string) (string, string, error) {
	importUrl, err := url.JoinPath(baseUrl, HTTP_ENDPOINT_IMPORT_URI)
	if err != nil {
		return "", "", err
	}
	healthUrl, err := url.JoinPath(baseUrl, HTTP_ENDPOINT_HEALTH_URI)
	if err != nil {
		return "", "", err
	}
	return importUrl, healthUrl, nil
}

func (eps *HttpEndpoints) AddHttpBaseUrl(baseUrl string) error {
	importUrl, healthUrl, err := HttpEndpointUrlsFromBaseUrl(baseUrl)
	if err != nil {
		return err
	}
	eps.AddHttpUrls(importUrl, healthUrl)
	return nil
}

// Retrieve the next URL to use in a LRU fashion:
func (eps *HttpEndpoints) GetImportUrl() string {
	eps.m.Lock()
	defer eps.m.Unlock()

	list := eps.healthy
	listElement := list.Front()
	if listElement == nil {
		return ""
	}
	list.MoveToBack(listElement)
	return listElement.Value.(*HttpEndpoint).importUrl
}

func (eps *HttpEndpoints) MarkImportUrlUnhealthy(importUrl string) *HttpEndpoint {
	eps.m.Lock()
	defer eps.m.Unlock()

	// Check against non-member or already marked:
	ep := eps.importUrlMap[importUrl]
	if ep == nil || ep.list == eps.unhealthy {
		return ep
	}
	ep.list.Remove(ep.listElement)
	ep.list = eps.unhealthy
	ep.listElement = ep.list.PushBack(ep)
	return ep
}

func (eps *HttpEndpoints) MarkHttpEndpointHealthy(ep *HttpEndpoint) {
	eps.m.Lock()
	defer eps.m.Unlock()

	if ep == nil {
		return
	}

	// Check against already marked:
	if ep.list == eps.healthy {
		return
	}
	ep.list.Remove(ep.listElement)
	ep.list = eps.healthy
	ep.listElement = ep.list.PushBack(ep)
}

func (eps *HttpEndpoints) PrintImportLists(w io.Writer) {
	eps.m.Lock()
	defer eps.m.Unlock()

	if w == nil {
		w = os.Stdout
	}

	fmt.Fprintln(w, "Healthy:")
	for e := eps.healthy.Front(); e != nil; e = e.Next() {
		fmt.Fprintln(w, " ", e.Value.(*HttpEndpoint).importUrl)
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Unhealthy:")
	for e := eps.unhealthy.Front(); e != nil; e = e.Next() {
		fmt.Fprintln(w, " ", e.Value.(*HttpEndpoint).importUrl)
	}
	fmt.Fprintln(w)
}
