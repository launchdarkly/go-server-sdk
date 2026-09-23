package ldotel

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	ldclient "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func fdv2StreamHandler() http.Handler {
	flag := ldbuilders.NewFlagBuilder("flag").SingleVariation(ldvalue.Bool(true)).Build()
	data := ldservicesv2.NewServerSDKData().Flags(flag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "fake-id", Target: 1, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)
	handler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
	return handler
}

func fdv2PollHandler() http.Handler {
	flag := ldbuilders.NewFlagBuilder("flag").SingleVariation(ldvalue.Bool(true)).Build()
	data := ldservicesv2.NewServerSDKData().Flags(flag)
	return ldservices.ServerSidePollingV2ServiceHandler(data.ToInitializerPayload(subsystems.NewSelector("state", 1)))
}

// waitForMetric collects until the named metric appears, or fails after the timeout.
func waitForMetric(t *testing.T, setup *metricsTestSetup, name string, timeout time.Duration) metricdata.ResourceMetrics {
	deadline := time.Now().Add(timeout)
	for {
		rm := setup.collect(t)
		if hasMetric(rm, name) {
			return rm
		}
		if time.Now().After(deadline) {
			require.Failf(t, "metric not found", "%s was not recorded within %s", name, timeout)
			return rm
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestMetricsHookRecordsInitializersAndSynchronizer(t *testing.T) {
	setup := newMetricsTestSetup(t)

	failingPoll := httptest.NewServer(httphelpers.HandlerWithStatus(http.StatusServiceUnavailable))
	defer failingPoll.Close()
	healthyPoll := httptest.NewServer(fdv2PollHandler())
	defer healthyPoll.Close()
	stream := httptest.NewServer(fdv2StreamHandler())
	defer stream.Close()

	client, err := ldclient.MakeCustomClient("sdk-key", ldclient.Config{
		Events: ldcomponents.NoEvents(),
		DataSystem: ldcomponents.DataSystem().Custom().
			Initializers(
				// A configured name tells two polling initializers apart in telemetry and logs.
				ldcomponents.PollingDataSourceV2().BaseURI(failingPoll.URL).Name("relay-poll").AsInitializer(),
				ldcomponents.PollingDataSourceV2().BaseURI(healthyPoll.URL).AsInitializer(),
			).
			Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(stream.URL)),
		Hooks: []ldhooks.Hook{setup.hook},
	}, 5*time.Second)
	require.NoError(t, err)
	closeClient := closeOnce(t, client)
	require.True(t, client.Initialized())

	// The synchronizer starts after initialization completes; wait for its first status.
	rm := waitForMetric(t, setup, metricSynchronizerActive, 5*time.Second)

	polling := []attribute.KeyValue{
		attribute.String(attrDataSourceName, "polling"),
		attribute.String(attrDataSourceProtocol, "fdv2"),
		attribute.String(attrDataSourceTransport, "polling"),
	}
	relayPolling := []attribute.KeyValue{
		attribute.String(attrDataSourceName, "relay-poll"),
		attribute.String(attrDataSourceProtocol, "fdv2"),
		attribute.String(attrDataSourceTransport, "polling"),
	}
	streaming := []attribute.KeyValue{
		attribute.String(attrDataSourceName, "streaming"),
		attribute.String(attrDataSourceProtocol, "fdv2"),
		attribute.String(attrDataSourceTransport, "streaming"),
	}

	attempts := requireMetric(t, rm, metricInitializerAttempts)
	assert.Equal(t, int64(1), sumInt64(t, attempts,
		append(relayPolling, attribute.String(attrInitializerOutcome, string(ldhooks.InitializerOutcomeFailed)))...))
	assert.Equal(t, int64(0), sumInt64(t, attempts,
		append(polling, attribute.String(attrInitializerOutcome, string(ldhooks.InitializerOutcomeFailed)))...))
	assert.Equal(t, int64(1), sumInt64(t, attempts,
		append(polling, attribute.String(attrInitializerOutcome, string(ldhooks.InitializerOutcomeSucceeded)))...))
	assert.Equal(t, int64(2), sumInt64(t, attempts))
	assert.Equal(t, uint64(2), histogramCount(t, requireMetric(t, rm, metricInitializerDuration)))

	initialization := requireMetric(t, rm, metricInitializationDuration)
	assert.Equal(t, uint64(1), histogramCount(t, initialization,
		append(polling, attribute.String(attrInitializationOutcome, initializationSucceeded))...))

	transitions := requireMetric(t, rm, metricSynchronizerTransitions)
	assert.Equal(t, int64(1), sumInt64(t, transitions,
		attribute.String(attrSynchronizerPrevious, noneValue),
		attribute.String(attrSynchronizerCurrent, "streaming"),
		attribute.String(attrSynchronizerReason, string(ldhooks.SynchronizerChangeReasonInitial)),
	))

	active, ok := lastGaugeInt64(t, requireMetric(t, rm, metricSynchronizerActive), streaming...)
	require.True(t, ok)
	assert.Equal(t, int64(1), active)

	// The initial INITIALIZING status arrived before any initializer, so it belongs to the data
	// system itself.
	stateTransitions := requireMetric(t, rm, metricDataSourceTransitions)
	assert.Equal(t, int64(1), sumInt64(t, stateTransitions,
		stateAttribute(interfaces.DataSourceStateInitializing),
		attribute.String(attrPreviousState, noneValue),
		attribute.String(attrDataSourceName, dataSystemName)))
	// The VALID status followed the applied initializer, so it belongs to that initializer.
	assert.Equal(t, int64(1), sumInt64(t, stateTransitions,
		append(polling, stateAttribute(interfaces.DataSourceStateValid))...))

	// The synchronizer change moves the state gauge: the streaming synchronizer now reads VALID and
	// every earlier component reads 0.
	state := requireMetric(t, rm, metricDataSourceState)
	streamValid, ok := lastGaugeInt64(t, state, append(streaming, stateAttribute(interfaces.DataSourceStateValid))...)
	require.True(t, ok)
	assert.Equal(t, int64(1), streamValid)
	pollValid, ok := lastGaugeInt64(t, state, append(polling, stateAttribute(interfaces.DataSourceStateValid))...)
	require.True(t, ok)
	assert.Equal(t, int64(0), pollValid)
	systemValid, ok := lastGaugeInt64(t, state,
		append(sourceAttributes(dataSystemDescriptor), stateAttribute(interfaces.DataSourceStateValid))...)
	require.True(t, ok)
	assert.Equal(t, int64(0), systemValid)

	// Closing the client reports OFF for the streaming synchronizer before the hook stops observing.
	closeClient()
	rm = setup.collect(t)
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceTransitions),
		append(streaming, stateAttribute(interfaces.DataSourceStateOff),
			attribute.String(attrPreviousState, string(interfaces.DataSourceStateValid)))...))
	assert.False(t, hasMetric(rm, metricDataSourceState))
	assert.False(t, hasMetric(rm, metricSynchronizerActive))
}

func TestMetricsHookRecordsSynchronizerRemovalAndExhaustion(t *testing.T) {
	setup := newMetricsTestSetup(t)

	// A 401 is unrecoverable: the only synchronizer is removed and none remain.
	stream := httptest.NewServer(httphelpers.HandlerWithStatus(http.StatusUnauthorized))
	defer stream.Close()

	client, err := ldclient.MakeCustomClient("sdk-key", ldclient.Config{
		Events: ldcomponents.NoEvents(),
		DataSystem: ldcomponents.DataSystem().Custom().
			Synchronizers(ldcomponents.StreamingDataSourceV2().BaseURI(stream.URL)),
		Hooks: []ldhooks.Hook{setup.hook},
	}, 5*time.Second)
	// A permanently failed data source makes client creation return an error along with a client.
	require.ErrorIs(t, err, ldclient.ErrInitializationFailed)
	require.NotNil(t, client)
	defer client.Close()
	require.False(t, client.Initialized())

	rm := waitForMetric(t, setup, metricInitializationDuration, 5*time.Second)

	transitions := requireMetric(t, rm, metricSynchronizerTransitions)
	assert.Equal(t, int64(1), sumInt64(t, transitions,
		attribute.String(attrSynchronizerCurrent, "streaming"),
		attribute.String(attrSynchronizerReason, string(ldhooks.SynchronizerChangeReasonInitial))))
	assert.Equal(t, int64(1), sumInt64(t, transitions,
		attribute.String(attrSynchronizerPrevious, "streaming"),
		attribute.String(attrSynchronizerCurrent, noneValue),
		attribute.String(attrSynchronizerReason, string(ldhooks.SynchronizerChangeReasonExhausted))))

	active, ok := lastGaugeInt64(t, requireMetric(t, rm, metricSynchronizerActive),
		attribute.String(attrDataSourceName, "streaming"))
	require.True(t, ok)
	assert.Equal(t, int64(0), active)

	initialization := requireMetric(t, rm, metricInitializationDuration)
	assert.Equal(t, uint64(1), histogramCount(t, initialization,
		attribute.String(attrInitializationOutcome, initializationFailed)))

	errors := requireMetric(t, rm, metricDataSourceErrors)
	assert.Equal(t, int64(1), sumInt64(t, errors,
		attribute.String(attrDataSourceName, "streaming"),
		attribute.Int(attrHTTPStatusCode, http.StatusUnauthorized)))
}
