// Endpoint spec list parser.
//
// ENDPOINT_LIST = ENDPOINT_SPEC[,ENDPOINT_SPEC]*
// ENDPOINT_SPEC = BASE_URL|(IMPORT_URL, HEALTH_URL)
//
//  e.g.
//	Only base URLs:
//		"http://1.1.1.1:8080,http://1.1.1.2:8080"
//  Explicit pairs:
//		"(http://1.1.1.1:8080/import,http://1.1.1.1:8080/health),(http://1.1.1.2:8080/import,http://1.1.1.2:8080/health)"
//  Mix:
//		"http://1.1.1.1:8080,(http://1.1.1.2:8080/import,http://1.1.1.2:8080/health)"

package pvmi

import (
	"fmt"
	"net/url"
	"unicode"
)

const (
	HTTP_ENDPOINT_HEALTH_URI = "/ready"
	HTTP_ENDPOINT_IMPORT_URI = "/api/v1/import/prometheus"
)

const (
	PARSE_URL_STATE_WAIT_FOR_NON_WS           = iota
	PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_START = iota
	PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_END   = iota
	PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_SEP   = iota
	PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_START = iota
	PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_END   = iota
	PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_SEP   = iota
	PARSE_URL_STATE_WAIT_FOR_BASE_URL_END     = iota
	PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP     = iota
)

type ImportHealthUrlPair struct {
	importUrl, healthUrl string
}

func HttpEndpointValidUrl(rawUrl string) error {
	parsedUrl, err := url.Parse(rawUrl)
	if err != nil {
		return nil
	}
	if parsedUrl.Scheme != "http" && parsedUrl.Scheme != "https" {
		return fmt.Errorf("%s: invalid scheme for `%s'", parsedUrl.Scheme, rawUrl)
	}
	if parsedUrl.Host == "" {
		return fmt.Errorf("missing host part for `%s'", rawUrl)
	}
	return nil
}

func HttpEndpointUrlsFromBaseUrl(baseUrl string) (*ImportHealthUrlPair, error) {
	err := HttpEndpointValidUrl(baseUrl)
	if err != nil {
		return nil, err
	}
	importUrl, err := url.JoinPath(baseUrl, HTTP_ENDPOINT_IMPORT_URI)
	if err != nil {
		return nil, err
	}
	healthUrl, err := url.JoinPath(baseUrl, HTTP_ENDPOINT_HEALTH_URI)
	if err != nil {
		return nil, err
	}
	return &ImportHealthUrlPair{importUrl, healthUrl}, nil
}

func ParseEndpointSpec(urlListSpec string) ([]*ImportHealthUrlPair, error) {
	urlPairList := make([]*ImportHealthUrlPair, 0)
	state, start, importUrl := PARSE_URL_STATE_WAIT_FOR_NON_WS, -1, ""
	for i, c := range urlListSpec {
		switch state {
		case PARSE_URL_STATE_WAIT_FOR_NON_WS:
			switch {
			case unicode.IsSpace(c):
			case c == '(':
				state = PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_START
			case unicode.IsLetter(c):
				state = PARSE_URL_STATE_WAIT_FOR_BASE_URL_END
				start = i
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		case PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_START:
			switch {
			case unicode.IsSpace(c):
			case unicode.IsLetter(c):
				state = PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_END
				start = i
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		case PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_END:
			switch {
			case unicode.IsSpace(c):
				state = PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_SEP
			case c == ',':
				state = PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_START
			case c == ')':
				return nil, fmt.Errorf("%c: Invalid character at index %d in `%s'", c, i, urlListSpec)
			}
			if state != PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_END {
				if start == i {
					return nil, fmt.Errorf("empty URL at index %d in `%s'", i, urlListSpec)
				}
				importUrl = urlListSpec[start:i]
				err := HttpEndpointValidUrl(importUrl)
				if err != nil {
					return nil, err
				}
			}
		case PARSE_URL_STATE_WAIT_FOR_IMPORT_URL_SEP:
			switch {
			case unicode.IsSpace(c):
			case c == ',':
				state = PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_START
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		case PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_START:
			switch {
			case unicode.IsSpace(c):
			case unicode.IsLetter(c):
				state = PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_END
				start = i
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		case PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_END:
			switch {
			case c == ')':
				state = PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP
			case unicode.IsSpace(c):
				state = PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_SEP
			case c == '(' || c == ',':
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
			if state != PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_END {
				if start == i {
					return nil, fmt.Errorf("empty URL at index %d in `%s'", i, urlListSpec)
				}
				healthUrl := urlListSpec[start:i]
				err := HttpEndpointValidUrl(healthUrl)
				if err != nil {
					return nil, err
				}
				urlPairList = append(urlPairList, &ImportHealthUrlPair{importUrl, healthUrl})
			}
		case PARSE_URL_STATE_WAIT_FOR_HEALTH_URL_SEP:
			switch {
			case unicode.IsSpace(c):
			case c == ')':
				state = PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		case PARSE_URL_STATE_WAIT_FOR_BASE_URL_END:
			switch {
			case unicode.IsSpace(c):
				state = PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP
			case c == ',':
				state = PARSE_URL_STATE_WAIT_FOR_NON_WS
			case c == ')' || c == '(':
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
			if state != PARSE_URL_STATE_WAIT_FOR_BASE_URL_END {
				if start == i {
					return nil, fmt.Errorf("empty URL at index %d in `%s'", i, urlListSpec)
				}
				urlPair, err := HttpEndpointUrlsFromBaseUrl(urlListSpec[start:i])
				if err != nil {
					return nil, err
				}
				err = HttpEndpointValidUrl(urlPair.importUrl)
				if err != nil {
					return nil, err
				}
				err = HttpEndpointValidUrl(urlPair.healthUrl)
				if err != nil {
					return nil, err
				}
				urlPairList = append(urlPairList, urlPair)
			}
		case PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP:
			switch {
			case unicode.IsSpace(c):
			case c == ',':
				state = PARSE_URL_STATE_WAIT_FOR_NON_WS
			default:
				return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", c, i, urlListSpec)
			}
		}
	}
	switch state {
	case PARSE_URL_STATE_WAIT_FOR_NON_WS, PARSE_URL_STATE_WAIT_FOR_BASE_URL_SEP:
	case PARSE_URL_STATE_WAIT_FOR_BASE_URL_END:
		if start == len(urlListSpec)-1 {
			return nil, fmt.Errorf("empty URL at index %d in `%s'", start, urlListSpec)
		}
		urlPair, err := HttpEndpointUrlsFromBaseUrl(urlListSpec[start:])
		if err != nil {
			return nil, err
		}
		err = HttpEndpointValidUrl(urlPair.importUrl)
		if err != nil {
			return nil, err
		}
		err = HttpEndpointValidUrl(urlPair.healthUrl)
		if err != nil {
			return nil, err
		}
		urlPairList = append(urlPairList, urlPair)
	default:
		return nil, fmt.Errorf("invalid character `%c' at index %d in `%s'", urlListSpec[start], start, urlListSpec)
	}
	return urlPairList, nil
}
