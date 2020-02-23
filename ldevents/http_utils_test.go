package ldevents

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strconv"
)

type delegatedTransport func(*http.Request) (*http.Response, error)

type httpRequestInfo struct {
	request *http.Request
	body    []byte
}

func (d delegatedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return d(req)
}

func newHTTPClientWithHandler(roundTripper func(*http.Request) (*http.Response, error)) *http.Client {
	return &http.Client{Transport: delegatedTransport(roundTripper)}
}

func newHTTPClientWithRequestSink(status int) (*http.Client, *[]*http.Request) {
	requests := &[]*http.Request{}
	return newHTTPClientWithHandler(func(request *http.Request) (*http.Response, error) {
		*requests = append(*requests, request)
		return newHTTPResponse(request, status, nil, nil), nil
	}), requests
}

func newHTTPResponse(request *http.Request, status int, headers http.Header, body []byte) *http.Response {
	resp := &http.Response{
		Status:     strconv.Itoa(status),
		StatusCode: status,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Request:    request,
		Header:     headers,
	}
	resp.Body = ioutil.NopCloser(bytes.NewBuffer(body))
	resp.ContentLength = int64(len(body))
	return resp
}

func getBody(request *http.Request) []byte {
	body, _ := ioutil.ReadAll(request.Body)
	return body
}
