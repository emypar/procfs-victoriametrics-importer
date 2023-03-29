// Unit tests for HTTP endpoints.

package pvmi

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/eparparita/procfs-victoriametrics-importer/testutils"
)

var HTTP_ENDPOINT_TEST_URLS = []string{
	"http://1.1.1.1:8029",
	"http://1.1.1.2:8029",
	"http://1.1.1.3:8029",
	"http://1.1.1.4:8029",
	"http://1.1.1.5:8029",
}

func TestHttpEndpointNewHttpEndpoints(t *testing.T) {
	eps := NewHttpEndpoints()

	for _, url := range HTTP_ENDPOINT_TEST_URLS {
		eps.AddHttpUrls(url, "")
	}

	if eps.healthy.Front() != nil {
		t.Fatal("Healthy list not empty after creation")
	}

	{
		want := ""
		got := eps.GetImportUrl()
		if want != got {
			t.Fatalf("GetImportUrl(): want: %#v, got: %#v", want, got)
		}
	}

	{
		want := testutils.DuplicateStrings(HTTP_ENDPOINT_TEST_URLS, true)
		got := make([]string, len(want))
		for i, e := 0, eps.unhealthy.Front(); e != nil; i, e = i+1, e.Next() {
			got[i] = e.Value.(*HttpEndpoint).importUrl
		}
		got = testutils.DuplicateStrings(got, true)

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestHttpEndpointMarkHttpEndpointHealthy(t *testing.T) {
	eps := NewHttpEndpoints()

	for _, url := range HTTP_ENDPOINT_TEST_URLS {
		eps.AddHttpUrls(url, "")
	}

	for i, url := range HTTP_ENDPOINT_TEST_URLS {
		ep := eps.importUrlMap[url]
		eps.MarkHttpEndpointHealthy(ep)

		want := testutils.DuplicateStrings(HTTP_ENDPOINT_TEST_URLS[:i+1], true)
		got := make([]string, len(want))
		for i, e := 0, eps.healthy.Front(); e != nil; i, e = i+1, e.Next() {
			got[i] = e.Value.(*HttpEndpoint).importUrl
		}
		got = testutils.DuplicateStrings(got, true)

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestHttpEndpointMarkImportUrlUnhealthy(t *testing.T) {
	eps := NewHttpEndpoints()

	for _, url := range HTTP_ENDPOINT_TEST_URLS {
		eps.AddHttpUrls(url, "")
	}

	for _, url := range HTTP_ENDPOINT_TEST_URLS {
		ep := eps.importUrlMap[url]
		eps.MarkHttpEndpointHealthy(ep)
	}

	for i, url := range HTTP_ENDPOINT_TEST_URLS {
		eps.MarkImportUrlUnhealthy(url)

		want := testutils.DuplicateStrings(HTTP_ENDPOINT_TEST_URLS[:i+1], true)
		got := make([]string, len(want))
		for i, e := 0, eps.unhealthy.Front(); e != nil; i, e = i+1, e.Next() {
			got[i] = e.Value.(*HttpEndpoint).importUrl
		}
		got = testutils.DuplicateStrings(got, true)

		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("mismatch (-want +got):\n%s", diff)
		}
	}
}

func testHttpEndpointGetImportUrl(t *testing.T, start, end int) {
	eps := NewHttpEndpoints()

	for _, url := range HTTP_ENDPOINT_TEST_URLS {
		eps.AddHttpUrls(url, "")
	}

	for i := start; i < end; i++ {
		ep := eps.importUrlMap[HTTP_ENDPOINT_TEST_URLS[i]]
		eps.MarkHttpEndpointHealthy(ep)
	}

	for i := 0; i < 3*(end-start)+1; i++ {
		want := HTTP_ENDPOINT_TEST_URLS[start+i%(end-start)]
		got := eps.GetImportUrl()
		if want != got {
			t.Fatalf("GetImportUrl(i#%d): want: %#v, got: %#v", i, want, got)
		}
	}
}

func TestHttpEndpointGetImportUrl(t *testing.T) {
	for start := 0; start < len(HTTP_ENDPOINT_TEST_URLS)-1; start++ {
		for end := start + 1; end < len(HTTP_ENDPOINT_TEST_URLS); end++ {
			t.Run(
				fmt.Sprintf("start=%d,end=%d", start, end),
				func(t *testing.T) {
					testHttpEndpointGetImportUrl(t, start, end)
				},
			)
		}
	}
}
