package core

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Header represents one key-value pair.
type Header struct {
	Key, Value string
}

// DoHTTP does an HTTP call, checks for standard failure cases, and
// returns the body contents.
func DoHTTP(
	cli *http.Client, url *url.URL, method string, body io.Reader,
	headers []Header, gRPC bool,
) ([]byte, error) {
	req, _ := http.NewRequest(method, url.String(), body)
	for _, hdr := range headers {
		req.Header.Add(hdr.Key, hdr.Value)
	}
	resp, err := cli.Do(req)

	if err != nil {
		return nil, err
	}

	if resp == nil {
		return nil, ErrServerResponseNone
	}
	if resp.Body == nil {
		return nil, ErrServerResponseEmpty
	}
	defer resp.Body.Close()

	content, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, ErrServerResponseReadFail
	}
	if resp.StatusCode != http.StatusOK {
		if gRPC {
			err = fmt.Errorf("bad HTTP status code: %d (%s)", resp.StatusCode, decodeGrpcStatus(content))
		} else {
			err = fmt.Errorf("bad HTTP status code: %d (%s)", resp.StatusCode, content)
		}
	}
	return content, err
}

// NewHTTPClient creates an http.Client with default parameters.
func NewHTTPClient() *http.Client {
	var dialcontext = net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	var transport = http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialcontext.DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{Transport: &transport, Timeout: time.Second * 3}
}
