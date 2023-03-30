package pvmi

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

type ParseEndpointSpecTestCase struct {
	urlListSpec string
	urlPairList []*ImportHealthUrlPair
	err         error
}

func (tc *ParseEndpointSpecTestCase) AddSpaces() *ParseEndpointSpecTestCase {
	newTc := ParseEndpointSpecTestCase{
		urlPairList: tc.urlPairList,
		err:         tc.err,
	}
	newTc.urlListSpec = "  " + strings.ReplaceAll(
		strings.ReplaceAll(
			strings.ReplaceAll(tc.urlListSpec, ",", " , "),
			"(", "  (  ",
		),
		")", "   )    ",
	) + "     "
	return &newTc
}

func testParseEndpointSpec(t *testing.T, tc *ParseEndpointSpecTestCase) {
	t.Logf("Testing %#v", tc.urlListSpec)
	urlPairList, err := ParseEndpointSpec(tc.urlListSpec)
	if tc.err != nil {
		if urlPairList != nil {
			buf := &bytes.Buffer{}
			for _, urlPair := range urlPairList {
				fmt.Fprintf(buf, " %#v\n", urlPair)
			}
			t.Fatalf("non nil list: got:\n%s", buf.String())
		} else if err == nil {
			t.Fatalf("%#v: want error: %v, got: %v", tc.urlListSpec, tc.err, err)
		} else if tc.err.Error() != err.Error() {
			t.Fatalf("%#v: want error: %v, got: %v", tc.urlListSpec, tc.err, err)
		}
	} else {
		if len(tc.urlPairList) != len(urlPairList) {
			t.Fatalf("%#v length mismatch: want %d, got: %d", tc.urlListSpec, len(tc.urlPairList), len(urlPairList))
		} else {
			for i, wantUrlPair := range tc.urlPairList {
				if *wantUrlPair != *urlPairList[i] {
					t.Fatalf("%#v[%d] mismatch: want: %#v, got: %#v", tc.urlListSpec, i, *wantUrlPair, *urlPairList[i])
					break
				}
			}
		}
	}
}

func TestParseEndpointBaseSpec(t *testing.T) {
	for _, tc := range []*ParseEndpointSpecTestCase{
		{
			"http://1.1.1.1:8087",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/api/v1/import/prometheus",
					"http://1.1.1.1:8087/ready",
				},
			},
			nil,
		},
		{
			"http://1.1.1.1:8087,http://1.1.1.2:8087",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/api/v1/import/prometheus",
					"http://1.1.1.1:8087/ready",
				},
				{
					"http://1.1.1.2:8087/api/v1/import/prometheus",
					"http://1.1.1.2:8087/ready",
				},
			},
			nil,
		},
		{
			"http://1.1.1.1:8087,http://1.1.1.2:8087,http://1.1.1.2:8088",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/api/v1/import/prometheus",
					"http://1.1.1.1:8087/ready",
				},
				{
					"http://1.1.1.2:8087/api/v1/import/prometheus",
					"http://1.1.1.2:8087/ready",
				},
				{
					"http://1.1.1.2:8088/api/v1/import/prometheus",
					"http://1.1.1.2:8088/ready",
				},
			},
			nil,
		},
	} {
		testParseEndpointSpec(t, tc)
		testParseEndpointSpec(t, tc.AddSpaces())
	}
}

func TestParseEndpointPairSpec(t *testing.T) {
	for _, tc := range []*ParseEndpointSpecTestCase{
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health)",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
			},
			nil,
		},
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health),(http://1.1.1.2:8087/import,http://1.1.1.2:8087/health)",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
				{
					"http://1.1.1.2:8087/import",
					"http://1.1.1.2:8087/health",
				},
			},
			nil,
		},
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health),(http://1.1.1.2:8087/import,http://1.1.1.2:8087/health),(http://1.1.1.2:8088/import,http://1.1.1.2:8088/health)",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
				{
					"http://1.1.1.2:8087/import",
					"http://1.1.1.2:8087/health",
				},
				{
					"http://1.1.1.2:8088/import",
					"http://1.1.1.2:8088/health",
				},
			},
			nil,
		},
	} {
		testParseEndpointSpec(t, tc)
		testParseEndpointSpec(t, tc.AddSpaces())
	}
}

func TestParseEndpointMixedSpec(t *testing.T) {
	for _, tc := range []*ParseEndpointSpecTestCase{
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health)",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
			},
			nil,
		},
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health),http://1.1.1.2:8087",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
				{
					"http://1.1.1.2:8087/api/v1/import/prometheus",
					"http://1.1.1.2:8087/ready",
				},
			},
			nil,
		},
		{
			"(http://1.1.1.1:8087/import,http://1.1.1.1:8087/health),http://1.1.1.2:8087,(http://1.1.1.2:8088/import,http://1.1.1.2:8088/health)",
			[]*ImportHealthUrlPair{
				{
					"http://1.1.1.1:8087/import",
					"http://1.1.1.1:8087/health",
				},
				{
					"http://1.1.1.2:8087/api/v1/import/prometheus",
					"http://1.1.1.2:8087/ready",
				},
				{
					"http://1.1.1.2:8088/import",
					"http://1.1.1.2:8088/health",
				},
			},
			nil,
		},
	} {
		testParseEndpointSpec(t, tc)
		testParseEndpointSpec(t, tc.AddSpaces())
	}
}

func TestParseEndpointEmptySpec(t *testing.T) {
	for _, tc := range []*ParseEndpointSpecTestCase{
		{
			"",
			[]*ImportHealthUrlPair{},
			nil,
		},
	} {
		testParseEndpointSpec(t, tc)
		testParseEndpointSpec(t, tc.AddSpaces())
	}
}

func TestParseEndpointErrorSpec(t *testing.T) {
	for _, specErrFmt := range [][2]string{
		{
			")",
			"invalid character `)' at index 0 in `%s'",
		},
		{
			",",
			"invalid character `,' at index 0 in `%s'",
		},
		{
			"htt://",
			"htt: invalid scheme for `%s'",
		},
	} {
		testParseEndpointSpec(
			t,
			&ParseEndpointSpecTestCase{
				specErrFmt[0],
				nil,
				fmt.Errorf(specErrFmt[1], specErrFmt[0]),
			},
		)
	}
}
