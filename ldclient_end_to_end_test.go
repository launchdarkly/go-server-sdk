package ldclient

import (
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	initializationFailedErrorMessage = "LaunchDarkly client initialization failed"
	pollingModeWarningMessage        = "You should only disable the streaming API if instructed to do so by LaunchDarkly support"
)

var (
	alwaysTrueFlag            = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(true)).Build()
	onlyTrueForImportantUsers = ldbuilders.NewFlagBuilder("always-true-flag").On(true).AddTarget(0, "important-user").Variations(ldvalue.Bool(true), ldvalue.Bool(false)).FallthroughVariation(1).Build()
	alwaysFalseFlag           = ldbuilders.NewFlagBuilder("always-true-flag").SingleVariation(ldvalue.Bool(false)).Build()
	testUser                  = lduser.NewUser("test-user-key")
)

// This file contains smoke tests for a complete SDK instance running against embedded HTTP servers. We have many
// component-level tests elsewhere (including tests of the components' network behavior using an instrumented
// HTTPClient), but the end-to-end tests verify that the client is setting those components up correctly, with a
// configuration that's as close to the default configuration as possible (just changing the service URIs).

func assertNoMoreRequests(t *testing.T, requestsCh <-chan httphelpers.HTTPRequestInfo) {
	assert.Equal(t, 0, len(requestsCh))
}

func TestDefaultDataSourceIsStreaming(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

func TestClientStartsInStreamingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	handler, requestsCh := httphelpers.RecordingHandler(streamHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.DumpIfTestFailed(t)

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
		assert.Len(t, logCapture.GetOutput(ldlog.Warn), 0)
	})
}

// Under the RETRY spec (SDK-2775), a 401 is no longer terminal: the client
// times out waiting for initial data but the data source keeps retrying
// indefinitely. Init returns the usual timeout error; the data source state is
// Interrupted (not Off); the stream keeps hitting the server. Replaces the
// pre-RETRY TestClientFailsToStartInStreamingModeWith401Error, which asserted
// the old permanent-stop behavior.
func TestClientInStreamingModeWith401KeepsRetrying(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		// Short reconnect delays so multiple attempts fit within the init wait window.
		// Extended-regime profile activates immediately on 401 (unexpected), so we need
		// to shorten its base too, not just the normal-regime InitialReconnectDelay.
		// Uses a test-local bypass because the SDK's public streaming builder
		// intentionally does not expose the extended-regime timing knobs.
		streamingBuilder := &compressedStreamingBuilder{
			initialReconnectDelay:         10 * time.Millisecond,
			extendedInitialReconnectDelay: 20 * time.Millisecond,
		}

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
			DataSource:       streamingBuilder,
		}

		client, err := MakeCustomClient(testSdkKey, config, 500*time.Millisecond)
		require.Error(t, err) // init timed out -- no permanent stop, no successful put either
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, ErrInitializationTimeout, err)

		// Under RETRY the SDK does not permanently stop on 401. Since we never
		// reached a Valid state, the SinkImpl keeps state as Initializing rather
		// than transitioning to Interrupted (see maybeUpdateStatus's Initializing
		// clamp), but LastError records the failure. The key assertion is
		// "state is NOT Off" -- no permanent stop.
		require.Eventually(t, func() bool {
			s := client.GetDataSourceStatusProvider().GetStatus()
			return s.LastError.Kind == interfaces.DataSourceErrorKindErrorResponse &&
				s.LastError.StatusCode == 401
		}, 2*time.Second, 10*time.Millisecond,
			"data source should record the 401 as LastError")
		assert.NotEqual(t, string(interfaces.DataSourceStateOff),
			string(client.GetDataSourceStatusProvider().GetStatus().State),
			"RETRY §1.2.1: no permanent stop on 401")

		// Flag evaluation still works and returns defaults.
		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		// Confirm the client kept retrying -- at least one additional request landed at the mock server.
		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		require.Eventually(t, func() bool { return len(requestsCh) >= 1 },
			500*time.Millisecond, 10*time.Millisecond,
			"expected the client to keep retrying after 401")
	})
}

func TestClientRetriesConnectionInStreamingModeWithNonFatalError(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	failThenSucceedHandler := httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(503), streamHandler)
	handler, requestsCh := httphelpers.RecordingHandler(failThenSucceedHandler)
	httphelpers.WithServer(handler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r0 := <-requestsCh
		assert.Equal(t, testSdkKey, r0.Request.Header.Get("Authorization"))
		r1 := <-requestsCh
		assert.Equal(t, testSdkKey, r1.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		expectedWarning := "Error in stream connection (will retry): HTTP error 503"
		assert.Equal(t, []string{expectedWarning}, logCapture.GetOutput(ldlog.Warn))
		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
	})
}

func TestClientStartsInPollingMode(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource:       ldcomponents.PollingDataSource(),
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Polling: pollServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		assert.Equal(t, string(interfaces.DataSourceStateValid), string(client.GetDataSourceStatusProvider().GetStatus().State))

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)

		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
		assertNoMoreRequests(t, requestsCh)

		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
		assert.Equal(t, []string{pollingModeWarningMessage}, logCapture.GetOutput(ldlog.Warn))
	})
}

// TestPollingRequestsCarryInstanceIDHeader asserts the spec-required SCMP
// X-LaunchDarkly-Instance-Id header is present on polling requests and that it is a v4 UUID.
func TestPollingRequestsCarryInstanceIDHeader(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			DataSource:       ldcomponents.PollingDataSource(),
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Polling: pollServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		r := <-requestsCh

		instanceID := r.Request.Header.Get("X-LaunchDarkly-Instance-Id")
		require.NotEmpty(t, instanceID, "X-LaunchDarkly-Instance-Id must be set on polling requests")
		parsed, parseErr := uuid.Parse(instanceID)
		require.NoError(t, parseErr, "instance id %q must be a parseable UUID", instanceID)
		assert.Equal(t, uuid.Version(4), parsed.Version(), "instance id must be UUID v4")
	})
}

// TestInstanceIDIsDifferentBetweenClients verifies the GUID is unique per SDK instance.
func TestInstanceIDIsDifferentBetweenClients(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	pollHandler, requestsCh := httphelpers.RecordingHandler(ldservices.ServerSidePollingServiceHandler(data))
	httphelpers.WithServer(pollHandler, func(pollServer *httptest.Server) {
		makeAndPoll := func() string {
			config := Config{
				DataSource:       ldcomponents.PollingDataSource(),
				Events:           ldcomponents.NoEvents(),
				Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
				ServiceEndpoints: interfaces.ServiceEndpoints{Polling: pollServer.URL},
			}
			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			r := <-requestsCh
			return r.Request.Header.Get("X-LaunchDarkly-Instance-Id")
		}

		id1 := makeAndPoll()
		id2 := makeAndPoll()
		assert.NotEmpty(t, id1)
		assert.NotEmpty(t, id2)
		assert.NotEqual(t, id1, id2, "each SDK instance should have a unique instance id")
	})
}

// Under the RETRY spec (SDK-2775), a 401 is no longer terminal for polling
// either: the goroutine keeps polling on the extended-regime cadence. Init
// times out; state is Interrupted; the client kept polling. Replaces the
// pre-RETRY TestClientFailsToStartInPollingModeWith401Error.
func TestClientInPollingModeWith401KeepsRetrying(t *testing.T) {
	handler, requestsCh := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(401))
	httphelpers.WithServer(handler, func(pollServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		pollingBuilder := ldcomponents.PollingDataSource()

		config := Config{
			DataSource:       pollingBuilder,
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Polling: pollServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, 500*time.Millisecond)
		require.Error(t, err) // init timed out
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, ErrInitializationTimeout, err)

		// Under RETRY the SDK does not permanently stop on 401. Since we never
		// reached a Valid state, the SinkImpl keeps state as Initializing rather
		// than transitioning to Interrupted. The key assertion is "state is NOT
		// Off" -- no permanent stop -- and LastError records the failure.
		require.Eventually(t, func() bool {
			s := client.GetDataSourceStatusProvider().GetStatus()
			return s.LastError.Kind == interfaces.DataSourceErrorKindErrorResponse &&
				s.LastError.StatusCode == 401
		}, 2*time.Second, 10*time.Millisecond,
			"data source should record the 401 as LastError")
		assert.NotEqual(t, string(interfaces.DataSourceStateOff),
			string(client.GetDataSourceStatusProvider().GetStatus().State),
			"RETRY §1.2.1: no permanent stop on 401")

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		// Confirm the polling goroutine hit the server. Not asserting a second
		// poll here -- the polling B1 wait floor is PollInterval (30s default),
		// so observing multiple polls at unit-test timescales requires the
		// internal-constructor pathway used by polling_data_source_test.go.
		r := <-requestsCh
		assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
	})
}

func TestClientSendsEventWithoutDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			logCapture := ldlogtest.NewMockLog()

			config := Config{
				DiagnosticOptOut: true,
				Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
				ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL, Events: eventsServer.URL},
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			client.Identify(testUser)
			client.Flush()

			r := <-eventRequestsCh
			assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
			assert.Equal(t, "/bulk", r.Request.URL.Path)
			assertNoMoreRequests(t, eventRequestsCh)

			var jsonValue ldvalue.Value
			err = json.Unmarshal(r.Body, &jsonValue)
			assert.NoError(t, err)
			assert.Equal(t, ldvalue.String("identify"), jsonValue.GetByIndex(0).GetByKey("kind"))
		})
	})
}

func TestClientSendsDiagnostics(t *testing.T) {
	eventsHandler, eventRequestsCh := httphelpers.RecordingHandler(ldservices.ServerSideEventsServiceHandler())
	httphelpers.WithServer(eventsHandler, func(eventsServer *httptest.Server) {
		data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
		streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
		httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
			config := Config{
				Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
				ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL, Events: eventsServer.URL},
			}

			client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
			require.NoError(t, err)
			defer client.Close()

			r := <-eventRequestsCh
			assert.Equal(t, testSdkKey, r.Request.Header.Get("Authorization"))
			assert.Equal(t, "/diagnostic", r.Request.URL.Path)
			var jsonValue ldvalue.Value
			err = json.Unmarshal(r.Body, &jsonValue)
			assert.NoError(t, err)
			assert.Equal(t, ldvalue.String("diagnostic-init"), jsonValue.GetByKey("kind"))
		})
	})
}

func TestClientUsesCustomTLSConfiguration(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())

	httphelpers.WithSelfSignedServer(streamHandler, func(server *httptest.Server, certData []byte, certs *x509.CertPool) {
		config := Config{
			Events:           ldcomponents.NoEvents(),
			HTTP:             ldcomponents.HTTPConfiguration().CACert(certData),
			Logging:          ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
		require.NoError(t, err)
		defer client.Close()

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.True(t, value)
	})
}

func TestClientStartupTimesOut(t *testing.T) {
	data := ldservices.NewServerSDKData().Flags(&alwaysTrueFlag)
	streamHandler, _ := ldservices.ServerSideStreamingServiceHandler(data.ToPutEvent())
	slowHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		streamHandler.ServeHTTP(w, r)
	})

	httphelpers.WithServer(slowHandler, func(streamServer *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:           ldcomponents.NoEvents(),
			Logging:          ldcomponents.Logging().Loggers(logCapture.Loggers),
			ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: streamServer.URL},
		}

		client, err := MakeCustomClient(testSdkKey, config, time.Millisecond*100)
		require.Error(t, err)
		require.NotNil(t, client)
		defer client.Close()

		assert.Equal(t, "timeout encountered waiting for LaunchDarkly client initialization", err.Error())

		value, _ := client.BoolVariation(alwaysTrueFlag.Key, testUser, false)
		assert.False(t, value)

		assert.Equal(t, []string{"Timeout encountered waiting for LaunchDarkly client initialization"}, logCapture.GetOutput(ldlog.Warn))
		assert.Len(t, logCapture.GetOutput(ldlog.Error), 0)
	})
}

// compressedStreamingBuilder is a test-only ComponentConfigurer that constructs
// the streaming data source with compressed retry timings. It bypasses the SDK's
// public builder because that builder intentionally does not expose the
// extended-regime timing knobs.
type compressedStreamingBuilder struct {
	initialReconnectDelay         time.Duration
	extendedInitialReconnectDelay time.Duration
}

func (b *compressedStreamingBuilder) Build(context subsystems.ClientContext) (subsystems.DataSource, error) {
	baseURI := endpoints.SelectBaseURI(
		context.GetServiceEndpoints(),
		endpoints.StreamingService,
		context.GetLogging().Loggers,
	)
	return datasource.NewStreamProcessor(context, context.GetDataSourceUpdateSink(), datasource.StreamConfig{
		URI:                           baseURI,
		InitialReconnectDelay:         b.initialReconnectDelay,
		ExtendedInitialReconnectDelay: b.extendedInitialReconnectDelay,
	}), nil
}
