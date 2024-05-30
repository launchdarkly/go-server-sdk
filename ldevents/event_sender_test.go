package ldevents

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
)

type errorInfo struct {
	status int
}

func (ei errorInfo) Handler() http.Handler {
	if ei.status > 0 {
		return httphelpers.HandlerWithStatus(ei.status)
	}
	return httphelpers.BrokenConnectionHandler()
}

func (ei errorInfo) String() string {
	if ei.status > 0 {
		return fmt.Sprintf("error %d", ei.status)
	}
	return "network error"
}

func TestDataIsSentToAnalyticsURI(t *testing.T) {
	es, requestsCh := makeEventSenderWithRequestSink()

	result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)
	assert.True(t, result.Success)

	assert.Equal(t, 1, len(requestsCh))
	r := <-requestsCh
	assert.Equal(t, fakeEventsURI, r.Request.URL.String())
	assert.Equal(t, arbitraryJSONData, r.Body)
}

func TestDataIsSentToDiagnosticURI(t *testing.T) {
	es, requestsCh := makeEventSenderWithRequestSink()

	result := es.SendEventData(DiagnosticEventDataKind, arbitraryJSONData, 1)
	assert.True(t, result.Success)

	assert.Equal(t, 1, len(requestsCh))
	r := <-requestsCh
	assert.Equal(t, fakeDiagnosticURI, r.Request.URL.String())
	assert.Equal(t, arbitraryJSONData, r.Body)
}

func TestUnknownDataKindIsIgnored(t *testing.T) {
	es, requestsCh := makeEventSenderWithRequestSink()

	result := es.SendEventData(EventDataKind("not valid"), arbitraryJSONData, 1)
	assert.False(t, result.Success)
	assert.False(t, result.MustShutDown)
	assert.Len(t, requestsCh, 0)
}

func TestAnalyticsEventsHaveSchemaAndPayloadIDHeaders(t *testing.T) {
	es, requestsCh := makeEventSenderWithRequestSink()

	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)
	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

	assert.Equal(t, 2, len(requestsCh))
	r0 := <-requestsCh
	r1 := <-requestsCh

	assert.Equal(t, currentEventSchema, r0.Request.Header.Get(eventSchemaHeader))
	assert.Equal(t, currentEventSchema, r1.Request.Header.Get(eventSchemaHeader))

	id0 := r0.Request.Header.Get(payloadIDHeader)
	id1 := r1.Request.Header.Get(payloadIDHeader)
	assert.NotEqual(t, "", id0)
	assert.NotEqual(t, "", id1)
	assert.NotEqual(t, id0, id1)
}

func TestDiagnosticEventsDoNotHaveSchemaOrPayloadID(t *testing.T) {
	es, requestsCh := makeEventSenderWithRequestSink()

	es.SendEventData(DiagnosticEventDataKind, arbitraryJSONData, 1)

	assert.Equal(t, 1, len(requestsCh))
	r := <-requestsCh
	assert.Equal(t, "", r.Request.Header.Get(eventSchemaHeader))
	assert.Equal(t, "", r.Request.Header.Get(payloadIDHeader))
}

func TestEventSenderParsesTimeFromServer(t *testing.T) {
	expectedTime := ldtime.UnixMillisFromTime(time.Date(1940, time.February, 15, 12, 13, 14, 0, time.UTC))
	headers := make(http.Header)
	headers.Set("Date", "Thu, 15 Feb 1940 12:13:14 GMT")
	handler := httphelpers.HandlerWithResponse(202, headers, nil)
	es := makeEventSenderWithHTTPClient(httphelpers.ClientFromHandler(handler))

	result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)
	assert.True(t, result.Success)
	assert.Equal(t, expectedTime, result.TimeFromServer)
}

func TestEventSenderRetriesOnRecoverableError(t *testing.T) {
	errs := []errorInfo{{400}, {408}, {429}, {500}, {503}, {0}}
	for _, errorInfo := range errs {
		t.Run(fmt.Sprintf("Retries once after %s", errorInfo), func(t *testing.T) {
			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.SequentialHandler(
					errorInfo.Handler(),                // fails once
					httphelpers.HandlerWithStatus(202), // then succeeds
				),
			)
			es := makeEventSenderWithHTTPClient(httphelpers.ClientFromHandler(handler))

			result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

			assert.True(t, result.Success)
			assert.False(t, result.MustShutDown)

			assert.Equal(t, 2, len(requestsCh))
			r0 := <-requestsCh
			r1 := <-requestsCh
			assert.Equal(t, arbitraryJSONData, r0.Body)
			assert.Equal(t, arbitraryJSONData, r1.Body)
			id0 := r0.Request.Header.Get(payloadIDHeader)
			assert.NotEqual(t, "", id0)
			assert.Equal(t, id0, r1.Request.Header.Get(payloadIDHeader))
		})

		t.Run(fmt.Sprintf("Does not retry more than once after %s", errorInfo), func(t *testing.T) {
			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.SequentialHandler(
					errorInfo.Handler(),                // fails once
					errorInfo.Handler(),                // fails again
					httphelpers.HandlerWithStatus(202), // then would succeed, if we did a 3rd request
				),
			)
			es := makeEventSenderWithHTTPClient(httphelpers.ClientFromHandler(handler))

			result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

			assert.False(t, result.Success)
			assert.False(t, result.MustShutDown)

			assert.Equal(t, 2, len(requestsCh))
			r0 := <-requestsCh
			r1 := <-requestsCh
			assert.Equal(t, arbitraryJSONData, r0.Body)
			assert.Equal(t, arbitraryJSONData, r1.Body)
			id0 := r0.Request.Header.Get(payloadIDHeader)
			assert.NotEqual(t, "", id0)
			assert.Equal(t, id0, r1.Request.Header.Get(payloadIDHeader))
		})
	}
}

func TestEventSenderFailsOnUnrecoverableError(t *testing.T) {
	errs := []errorInfo{{401}, {403}}
	for _, errorInfo := range errs {
		t.Run(fmt.Sprintf("Fails permanently after %s", errorInfo), func(t *testing.T) {
			handler, requestsCh := httphelpers.RecordingHandler(
				httphelpers.SequentialHandler(
					errorInfo.Handler(),                // fails once
					httphelpers.HandlerWithStatus(202), // then succeeds
				),
			)
			es := makeEventSenderWithHTTPClient(httphelpers.ClientFromHandler(handler))

			result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

			assert.False(t, result.Success)
			assert.True(t, result.MustShutDown)

			assert.Equal(t, 1, len(requestsCh))
			r := <-requestsCh
			assert.Equal(t, arbitraryJSONData, r.Body)
		})
	}
}

func TestEventSenderDoesNotShutdownOnLargePayload(t *testing.T) {
	errorInfo := errorInfo{413}
	t.Run(fmt.Sprintf("Fails permanently after %s", errorInfo), func(t *testing.T) {
		handler, _ := httphelpers.RecordingHandler(
			httphelpers.SequentialHandler(errorInfo.Handler()),
		)
		es := makeEventSenderWithHTTPClient(httphelpers.ClientFromHandler(handler))

		result := es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

		assert.False(t, result.Success)
		assert.False(t, result.MustShutDown)
	})
}

func TestServerSideSenderSetsURIsFromBase(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(202))
	client := httphelpers.ClientFromHandler(handler)
	es := NewServerSideEventSender(EventSenderConfiguration{Client: client, BaseURI: fakeBaseURI, Loggers: ldlog.NewDisabledLoggers()},
		sdkKey)

	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)
	es.SendEventData(DiagnosticEventDataKind, arbitraryJSONData, 1)

	assert.Equal(t, 2, len(requestsCh))
	r0 := <-requestsCh
	r1 := <-requestsCh
	assert.Equal(t, fakeEventsURI, r0.Request.URL.String())
	assert.Equal(t, fakeDiagnosticURI, r1.Request.URL.String())
}

func TestServerSideSenderHasDefaultBaseURI(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(202))
	client := httphelpers.ClientFromHandler(handler)
	es := NewServerSideEventSender(
		EventSenderConfiguration{
			Client:  client,
			Loggers: ldlog.NewDisabledLoggers(),
		},
		sdkKey,
	)

	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)
	es.SendEventData(DiagnosticEventDataKind, arbitraryJSONData, 1)

	assert.Equal(t, 2, len(requestsCh))
	r0 := <-requestsCh
	r1 := <-requestsCh
	assert.Equal(t, "https://events.launchdarkly.com/bulk", r0.Request.URL.String())
	assert.Equal(t, "https://events.launchdarkly.com/diagnostic", r1.Request.URL.String())
}

func TestServerSideSenderAddsAuthorizationHeader(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(202))
	client := httphelpers.ClientFromHandler(handler)
	extraHeaders := make(http.Header)
	extraHeaders.Set("my-header", "my-value")
	es := NewServerSideEventSender(
		EventSenderConfiguration{
			Client:      client,
			BaseURI:     fakeBaseURI,
			BaseHeaders: func() http.Header { return extraHeaders },
			Loggers:     ldlog.NewDisabledLoggers(),
		},
		sdkKey,
	)

	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

	assert.Equal(t, 1, len(requestsCh))
	r := <-requestsCh
	assert.Equal(t, sdkKey, r.Request.Header.Get("Authorization"))
	assert.Equal(t, "my-value", r.Request.Header.Get("my-header"))
}

func TestSchemaVersionCannotBeOverriddenWithServerSideSender(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerForMethod("POST", httphelpers.HandlerWithStatus(202), nil))
	client := httphelpers.ClientFromHandler(handler)

	config := EventSenderConfiguration{Client: client, SchemaVersion: 99}
	es := makeEventSenderWithConfig(config)

	es.SendEventData(AnalyticsEventDataKind, arbitraryJSONData, 1)

	r := <-requestsCh
	assert.Equal(t, currentEventSchema, r.Request.Header.Get(eventSchemaHeader))
}

func TestSchemaVersionCanBeOverriddenWithDirectSend(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerForMethod("POST", httphelpers.HandlerWithStatus(202), nil))
	client := httphelpers.ClientFromHandler(handler)

	config := EventSenderConfiguration{Client: client, SchemaVersion: 99}

	_ = SendEventDataWithRetry(config, AnalyticsEventDataKind, "", arbitraryJSONData, 1)

	r := <-requestsCh
	assert.Equal(t, "/bulk", r.Request.URL.Path)
	assert.Equal(t, "99", r.Request.Header.Get(eventSchemaHeader))
}

func TestSendEventDataCanUseDefaultHTTPClient(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerForMethod("POST", httphelpers.HandlerWithStatus(202), nil))
	server := httptest.NewServer(handler)
	defer server.Close()

	config := EventSenderConfiguration{BaseURI: server.URL}

	_ = SendEventDataWithRetry(config, AnalyticsEventDataKind, "", arbitraryJSONData, 1)

	r := <-requestsCh
	assert.Equal(t, "/bulk", r.Request.URL.Path)
	assert.Equal(t, string(arbitraryJSONData), string(r.Body))
}

func TestSendEventDataCanOverrideURI(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerForMethod("POST", httphelpers.HandlerWithStatus(202), nil))
	server := httptest.NewServer(handler)
	defer server.Close()

	config := EventSenderConfiguration{BaseURI: server.URL}

	_ = SendEventDataWithRetry(config, AnalyticsEventDataKind, "/other/path", arbitraryJSONData, 1)
	_ = SendEventDataWithRetry(config, AnalyticsEventDataKind, "other/path", arbitraryJSONData, 1)

	r1, r2 := <-requestsCh, <-requestsCh
	assert.Equal(t, "/other/path", r1.Request.URL.Path)
	assert.Equal(t, "/other/path", r2.Request.URL.Path)
}

func makeEventSenderWithConfig(config EventSenderConfiguration) EventSender {
	config.BaseURI = fakeBaseURI
	config.Loggers = ldlog.NewDisabledLoggers()
	if config.RetryDelay == 0 {
		config.RetryDelay = briefRetryDelay
	}
	return NewServerSideEventSender(config, sdkKey)
}

func makeEventSenderWithHTTPClient(client *http.Client) EventSender {
	return makeEventSenderWithConfig(EventSenderConfiguration{Client: client})
}

func makeEventSenderWithRequestSink() (EventSender, <-chan httphelpers.HTTPRequestInfo) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerForMethod("POST", httphelpers.HandlerWithStatus(202), nil))
	client := httphelpers.ClientFromHandler(handler)
	return makeEventSenderWithHTTPClient(client), requestsCh
}
