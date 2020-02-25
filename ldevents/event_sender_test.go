package ldevents

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

const (
	fakeBaseURI       = "https://fake-server"
	fakeEventsURI     = fakeBaseURI + "/bulk"
	fakeDiagnosticURI = fakeBaseURI + "/diagnostic"
	briefRetryDelay   = 50 * time.Millisecond
)

var fakeEventData = []byte("hello")

type errorInfo struct {
	status int
	err    error
}

func (ei errorInfo) String() string {
	if ei.err == nil {
		return fmt.Sprintf("error %d", ei.status)
	}
	return "network error"
}

func TestDataIsSentToAnalyticsURI(t *testing.T) {
	es, requests := makeEventSenderWithRequestSink()

	result := es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)
	assert.True(t, result.Success)

	assert.Equal(t, 1, len(*requests))
	request := (*requests)[0]
	assert.Equal(t, fakeEventsURI, request.URL.String())
	assert.Equal(t, fakeEventData, getBody(request))
}

func TestDataIsSentToDiagnosticURI(t *testing.T) {
	es, requests := makeEventSenderWithRequestSink()

	result := es.SendEventData(DiagnosticEventDataKind, fakeEventData, 1)
	assert.True(t, result.Success)

	assert.Equal(t, 1, len(*requests))
	request := (*requests)[0]
	assert.Equal(t, fakeDiagnosticURI, request.URL.String())
	assert.Equal(t, fakeEventData, getBody(request))
}

func TestAnalyticsEventsHaveSchemaAndPayloadIDHeaders(t *testing.T) {
	es, requests := makeEventSenderWithRequestSink()

	es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)
	es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)

	assert.Equal(t, 2, len(*requests))
	request0 := (*requests)[0]
	request1 := (*requests)[1]

	assert.Equal(t, currentEventSchema, request0.Header.Get(eventSchemaHeader))
	assert.Equal(t, currentEventSchema, request1.Header.Get(eventSchemaHeader))

	id0 := request0.Header.Get(payloadIDHeader)
	id1 := request1.Header.Get(payloadIDHeader)
	assert.NotEqual(t, "", id0)
	assert.NotEqual(t, "", id1)
	assert.NotEqual(t, id0, id1)
}

func TestDiagnosticEventsDoNotHaveSchemaOrPayloadID(t *testing.T) {
	es, requests := makeEventSenderWithRequestSink()

	es.SendEventData(DiagnosticEventDataKind, fakeEventData, 1)

	assert.Equal(t, 1, len(*requests))
	request := (*requests)[0]
	assert.Equal(t, "", request.Header.Get(eventSchemaHeader))
	assert.Equal(t, "", request.Header.Get(payloadIDHeader))
}

func TestEventSenderParsesTimeFromServer(t *testing.T) {
	expectedTime := ldtime.UnixMillisFromTime(time.Date(1940, time.February, 15, 12, 13, 14, 0, time.UTC))
	client := newHTTPClientWithHandler(func(request *http.Request) (*http.Response, error) {
		headers := make(http.Header)
		headers.Set("Date", "Thu, 15 Feb 1940 12:13:14 GMT")
		return newHTTPResponse(request, 202, headers, nil), nil
	})
	es := makeEventSenderWithHTTPClient(client)

	result := es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)
	assert.True(t, result.Success)
	assert.Equal(t, expectedTime, result.TimeFromServer)
}

func TestEventSenderRetriesOnRecoverableError(t *testing.T) {
	errs := []errorInfo{{400, nil}, {408, nil}, {429, nil}, {500, nil}, {503, nil}, {0, errors.New("fake network error")}}
	for _, errorInfo := range errs {
		t.Run(fmt.Sprintf("Retries once after %s", errorInfo), func(t *testing.T) {
			var requests []*http.Request
			client := newHTTPClientWithHandler(httpHandlerThatFailsTimes(1, errorInfo, 202, &requests))
			es := makeEventSenderWithHTTPClient(client)

			result := es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)

			assert.True(t, result.Success)
			assert.False(t, result.MustShutDown)

			assert.Equal(t, 2, len(requests))
			assert.Equal(t, fakeEventData, getBody(requests[0]))
			assert.Equal(t, fakeEventData, getBody(requests[1]))
			id0 := requests[0].Header.Get(payloadIDHeader)
			assert.NotEqual(t, "", id0)
			assert.Equal(t, id0, requests[1].Header.Get(payloadIDHeader))
		})

		t.Run(fmt.Sprintf("Does not retry more than once after %s", errorInfo), func(t *testing.T) {
			var requests []*http.Request
			client := newHTTPClientWithHandler(httpHandlerThatFailsTimes(2, errorInfo, 202, &requests))
			es := makeEventSenderWithHTTPClient(client)

			result := es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)

			assert.False(t, result.Success)
			assert.False(t, result.MustShutDown)

			assert.Equal(t, 2, len(requests))
			assert.Equal(t, fakeEventData, getBody(requests[0]))
			assert.Equal(t, fakeEventData, getBody(requests[1]))
			id0 := requests[0].Header.Get(payloadIDHeader)
			assert.NotEqual(t, "", id0)
			assert.Equal(t, id0, requests[1].Header.Get(payloadIDHeader))
		})
	}
}

func TestEventSenderFailsOnUnrecoverableError(t *testing.T) {
	errs := []errorInfo{{401, nil}, {403, nil}}
	for _, errorInfo := range errs {
		t.Run(fmt.Sprintf("Fails permanently after %s", errorInfo), func(t *testing.T) {
			var requests []*http.Request
			client := newHTTPClientWithHandler(httpHandlerThatFailsTimes(1, errorInfo, 202, &requests))
			es := makeEventSenderWithHTTPClient(client)

			result := es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)

			assert.False(t, result.Success)
			assert.True(t, result.MustShutDown)

			assert.Equal(t, 1, len(requests))
			assert.Equal(t, fakeEventData, getBody(requests[0]))
		})
	}
}

func TestServerSideSenderSetsURIsFromBase(t *testing.T) {
	client, requests := newHTTPClientWithRequestSink(202)
	es := NewServerSideEventSender(client, sdkKey, fakeBaseURI, nil, ldlog.NewDisabledLoggers())

	es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)
	es.SendEventData(DiagnosticEventDataKind, fakeEventData, 1)

	assert.Equal(t, 2, len(*requests))
	request0 := (*requests)[0]
	request1 := (*requests)[1]
	assert.Equal(t, fakeEventsURI, request0.URL.String())
	assert.Equal(t, fakeDiagnosticURI, request1.URL.String())
}

func TestServerSideSenderHasDefaultBaseURI(t *testing.T) {
	client, requests := newHTTPClientWithRequestSink(202)
	es := NewServerSideEventSender(client, sdkKey, "", nil, ldlog.NewDisabledLoggers())

	es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)
	es.SendEventData(DiagnosticEventDataKind, fakeEventData, 1)

	assert.Equal(t, 2, len(*requests))
	request0 := (*requests)[0]
	request1 := (*requests)[1]
	assert.Equal(t, "https://events.launchdarkly.com/bulk", request0.URL.String())
	assert.Equal(t, "https://events.launchdarkly.com/diagnostic", request1.URL.String())
}

func TestServerSideSenderAddsAuthorizationHeader(t *testing.T) {
	extraHeaders := make(http.Header)
	extraHeaders.Set("my-header", "my-value")
	client, requests := newHTTPClientWithRequestSink(202)
	es := NewServerSideEventSender(client, sdkKey, fakeBaseURI, extraHeaders, ldlog.NewDisabledLoggers())

	es.SendEventData(AnalyticsEventDataKind, fakeEventData, 1)

	assert.Equal(t, 1, len(*requests))
	request := (*requests)[0]
	assert.Equal(t, sdkKey, request.Header.Get("Authorization"))
	assert.Equal(t, "my-value", request.Header.Get("my-header"))
}

func makeEventSenderWithHTTPClient(client *http.Client) EventSender {
	return &defaultEventSender{
		httpClient:    client,
		eventsURI:     fakeEventsURI,
		diagnosticURI: fakeDiagnosticURI,
		loggers:       ldlog.NewDisabledLoggers(),
		retryDelay:    briefRetryDelay,
	}
}

func makeEventSenderWithRequestSink() (EventSender, *[]*http.Request) {
	client, requests := newHTTPClientWithRequestSink(202)
	return makeEventSenderWithHTTPClient(client), requests
}

func httpHandlerThatFailsTimes(times int, ei errorInfo, finalStatus int, requestsOut *[]*http.Request) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		*requestsOut = append(*requestsOut, req)
		if len(*requestsOut) > times {
			return newHTTPResponse(req, finalStatus, nil, nil), nil
		}
		if ei.err != nil {
			return nil, ei.err
		}
		return newHTTPResponse(req, ei.status, nil, nil), nil
	}
}
