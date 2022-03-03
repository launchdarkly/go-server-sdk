package ldclient

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/endpoints"
	"gopkg.in/launchdarkly/go-server-sdk.v6/ldcomponents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests verify that the SDK is using the expected base URIs in various configurations.
// Since we need to be able to intercept requests that would normally go to the production service
// endpoints, and we don't care about simulating realistic responses, we'll use a fake HTTP client
// rather than an embedded server.

type recordingClientFactory struct {
	requestURLs chan url.URL
	status      int
}

const customURI = "http://custom/"

func mustParseURI(s string) url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return *u
}

func baseURIOf(u url.URL) url.URL {
	u.Path = "/"
	return u
}

func newRecordingClientFactory(status int) *recordingClientFactory {
	return &recordingClientFactory{status: status, requestURLs: make(chan url.URL, 100)}
}

func (r *recordingClientFactory) MakeClient() *http.Client {
	c := *http.DefaultClient
	c.Transport = r
	return &c
}

func (r *recordingClientFactory) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requestURLs <- *req.URL
	return &http.Response{
		StatusCode: r.status,
		Body:       ioutil.NopCloser(bytes.NewBuffer(nil)),
	}, nil
}

func (r *recordingClientFactory) requireRequest(t *testing.T) url.URL {
	select {
	case u := <-r.requestURLs:
		return u
	case <-time.After(time.Second):
		require.Fail(t, "timed out waiting for request")
		return url.URL{}
	}
}

func TestDefaultStreamingDataSourceBaseUri(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		Events: ldcomponents.NoEvents(),
		HTTP:   ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(endpoints.DefaultStreamingBaseURI), baseURIOf(u))
}

func TestDefaultPollingDataSourceBaseUri(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		DataSource: ldcomponents.PollingDataSource(),
		Events:     ldcomponents.NoEvents(),
		HTTP:       ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(endpoints.DefaultPollingBaseURI), baseURIOf(u))
}

func TestDefaultEventsBaseUri(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		HTTP:       ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(endpoints.DefaultEventsBaseURI), baseURIOf(u))
}

func TestCustomStreamingBaseURI(t *testing.T) {
	rec := newRecordingClientFactory(401)
	mockLog := ldlogtest.NewMockLog()
	config := Config{
		Events:           ldcomponents.NoEvents(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: customURI},
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()

	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))

	mockLog.AssertMessageMatch(t, false, ldlog.Error, "You have set custom ServiceEndpoints without specifying")
}

func TestCustomPollingBaseURI(t *testing.T) {
	rec := newRecordingClientFactory(401)
	mockLog := ldlogtest.NewMockLog()
	config := Config{
		DataSource:       ldcomponents.PollingDataSource(),
		Events:           ldcomponents.NoEvents(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Polling: customURI},
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()

	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))

	mockLog.AssertMessageMatch(t, false, ldlog.Error, "You have set custom ServiceEndpoints without specifying")
}

func TestCustomEventsBaseURI(t *testing.T) {
	rec := newRecordingClientFactory(401)
	mockLog := ldlogtest.NewMockLog()
	config := Config{
		DataSource:       ldcomponents.ExternalUpdatesOnly(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Events: customURI},
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()

	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))

	mockLog.AssertMessageMatch(t, false, ldlog.Error, "You have set custom ServiceEndpoints without specifying")
}

func TestErrorIsLoggedIfANecessaryURIIsNotSetWhenOtherCustomURIsAreSet(t *testing.T) {
	rec := newRecordingClientFactory(401)

	mockLog1 := ldlogtest.NewMockLog()
	config1 := Config{
		Events:           ldcomponents.NoEvents(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog1.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Events: customURI},
	}
	client1, _ := MakeCustomClient(testSdkKey, config1, time.Second*5)
	require.NotNil(t, client1)
	client1.Close()
	mockLog1.AssertMessageMatch(t, true, ldlog.Error,
		"You have set custom ServiceEndpoints without specifying the Streaming base URI")

	mockLog2 := ldlogtest.NewMockLog()
	config2 := Config{
		DataSource:       ldcomponents.PollingDataSource(),
		Events:           ldcomponents.NoEvents(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog2.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: customURI},
	}
	client2, _ := MakeCustomClient(testSdkKey, config2, time.Second*5)
	require.NotNil(t, client2)
	client2.Close()
	mockLog2.AssertMessageMatch(t, true, ldlog.Error,
		"You have set custom ServiceEndpoints without specifying the Polling base URI")

	mockLog3 := ldlogtest.NewMockLog()
	config3 := Config{
		DataSource:       ldcomponents.ExternalUpdatesOnly(),
		HTTP:             ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
		Logging:          ldcomponents.Logging().Loggers(mockLog3.Loggers),
		ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: customURI},
	}
	client3, _ := MakeCustomClient(testSdkKey, config3, time.Second*5)
	require.NotNil(t, client3)
	client3.Close()
	mockLog3.AssertMessageMatch(t, true, ldlog.Error,
		"You have set custom ServiceEndpoints without specifying the Events base URI")
}

func TestCustomStreamingBaseURIWithDeprecatedMethod(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		DataSource: ldcomponents.StreamingDataSource().BaseURI(customURI),
		Events:     ldcomponents.NoEvents(),
		HTTP:       ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))
}

func TestCustomPollingBaseURIWithDeprecatedMethod(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		DataSource: ldcomponents.PollingDataSource().BaseURI(customURI),
		Events:     ldcomponents.NoEvents(),
		HTTP:       ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))
}

func TestCustomEventsBaseURIWithDeprecatedMethod(t *testing.T) {
	rec := newRecordingClientFactory(401)
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		Events:     ldcomponents.SendEvents().BaseURI(customURI),
		HTTP:       ldcomponents.HTTPConfiguration().HTTPClientFactory(rec.MakeClient),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NotNil(t, client)
	defer client.Close()
	u := rec.requireRequest(t)
	assert.Equal(t, mustParseURI(customURI), baseURIOf(u))
}
