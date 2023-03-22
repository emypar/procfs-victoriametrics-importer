// VictoriaMetrics import endpoint emulator

package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

var displayRequest, displayHeaders, displayBody bool

func handlerFunc(_ http.ResponseWriter, r *http.Request) {
	ts := float64(time.Now().UnixMicro()) / 1_000_000.
	rSize := 0

	var err error

	rSize += len(r.Method) + len(r.RequestURI) + len(r.Proto)

	var body []byte
	if r.Method == "PUT" || r.Method == "POST" {
		body, err = io.ReadAll(r.Body)
		if err == nil {
			rSize += len(body)
		}
	}

	_, ok := r.Header["Transfer-Encoding"]
	if !ok && len(r.TransferEncoding) > 0 {
		r.Header["Transfer-Encoding"] = r.TransferEncoding
	}
	_, ok = r.Header["Content-Length"]
	if !ok && r.ContentLength > 0 {
		r.Header["Content-Length"] = []string{
			fmt.Sprintf("%d", r.ContentLength),
		}
	}
	isText := true
	for hdr, hdrVals := range r.Header {
		rSize += len(hdr)
		for _, val := range hdrVals {
			rSize += len(val)
		}
		switch hdr {
		case "Content-Encoding":
			if body != nil {
				for _, val := range hdrVals {
					switch val {
					case "gzip":
						b := bytes.NewBuffer(body)
						var gzipReader *gzip.Reader
						gzipReader, err = gzip.NewReader(b)
						if err == nil {
							body, err = io.ReadAll(gzipReader)
						}
					case "":
					default:
						err = fmt.Errorf("%s: unsupported encoding", val)
					}
					if err != nil {
						break
					}
				}
			}
		case "Content-Type":
			isText = false
			for _, val := range hdrVals {
				if (len(val) >= 5 && val[:5] == "text/") ||
					val == "application/x-www-form-urlencoded" {
					isText = true
					break
				}
			}
		}
	}

	auditTrail := fmt.Sprintf("%.06f,%d\n", ts, rSize)

	buf := &bytes.Buffer{}
	if err != nil || displayRequest {
		fmt.Fprintf(
			buf,
			"%s %s %s %s\n",
			r.RemoteAddr,
			r.Method,
			r.RequestURI,
			r.Proto,
		)
	}
	if err != nil || displayHeaders {
		for hdr, hdrVals := range r.Header {
			fmt.Fprintf(buf, "%s: %s\n", hdr, strings.Join(hdrVals, ", "))
		}
		buf.WriteByte('\n')
	}
	if err != nil {
		fmt.Fprintf(buf, "Error decoding request: %s\n", err)
	} else {
		if displayHeaders {
			fmt.Fprintf(buf, "Body length: %d bytes after decoding\n", len(body))
		}
		if body != nil && isText && displayBody {
			buf.WriteByte('\n')
			buf.Write(body)
			buf.WriteByte('\n')
		}
		if displayRequest {
			fmt.Fprintf(buf, "Audit trail (TS,SIZE): %s", auditTrail)
		}
	}
	if buf.Len() > 0 {
		buf.WriteByte('\n')
		log.Print(buf)
	}
}

func main() {
	displayDetail := flag.String(
		"display-detail",
		"",
		"Select the detail to log: request, headers, body",
	)

	port := flag.Int(
		"port",
		8080,
		"The port to use",
	)

	flag.Parse()

	switch *displayDetail {
	case "request":
		displayRequest = true
	case "headers":
		displayRequest = true
		displayHeaders = true
	case "body":
		displayRequest = true
		displayHeaders = true
		displayBody = true
	}

	http.HandleFunc("/", handlerFunc)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
