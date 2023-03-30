package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		return
	}

	rawUrl := args[0]
	url, err := url.Parse(rawUrl)
	if err != nil {
		fmt.Println(err)
		return
	}

	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   2 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   5 * time.Second,
	}
	request := &http.Request{
		Method: http.MethodHead,
		URL:    url,
	}
	response, err := client.Do(request)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(response.Status)
	for hdr, hdrVals := range response.Header {
		fmt.Printf("%s: %s\n", hdr, strings.Join(hdrVals, ", "))
	}
	fmt.Println()

}
