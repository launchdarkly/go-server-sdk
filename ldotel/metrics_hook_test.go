package ldotel

import (
	gocontext "context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	ldclient "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

type metricsTestSetup struct {
	reader *sdkmetric.ManualReader
	hook   *MetricsHook
}

func newMetricsTestSetup(t *testing.T) *metricsTestSetup {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	hook, err := NewMetricsHook(WithMeterProvider(provider))
	require.NoError(t, err)
	return &metricsTestSetup{reader: reader, hook: hook}
}

func (s *metricsTestSetup) collect(t *testing.T) metricdata.ResourceMetrics {
	var rm metricdata.ResourceMetrics
	require.NoError(t, s.reader.Collect(gocontext.Background(), &rm))
	return rm
}

func requireMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m
			}
		}
	}
	require.Failf(t, "metric not found", "no metric named %s was recorded", name)
	return metricdata.Metrics{}
}

func hasMetric(rm metricdata.ResourceMetrics, name string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}

func hasAttributes(set attribute.Set, want []attribute.KeyValue) bool {
	for _, kv := range want {
		v, ok := set.Value(kv.Key)
		if !ok || v.Emit() != kv.Value.Emit() {
			return false
		}
	}
	return true
}

// sumInt64 totals the data points of an int64 sum whose attributes include all of want.
func sumInt64(t *testing.T, m metricdata.Metrics, want ...attribute.KeyValue) int64 {
	sum, ok := m.Data.(metricdata.Sum[int64])
	require.True(t, ok, "%s is not an int64 sum", m.Name)
	var total int64
	for _, dp := range sum.DataPoints {
		if hasAttributes(dp.Attributes, want) {
			total += dp.Value
		}
	}
	return total
}

func sumFloat64(t *testing.T, m metricdata.Metrics, want ...attribute.KeyValue) float64 {
	sum, ok := m.Data.(metricdata.Sum[float64])
	require.True(t, ok, "%s is not a float64 sum", m.Name)
	var total float64
	for _, dp := range sum.DataPoints {
		if hasAttributes(dp.Attributes, want) {
			total += dp.Value
		}
	}
	return total
}

func lastGaugeInt64(t *testing.T, m metricdata.Metrics, want ...attribute.KeyValue) (int64, bool) {
	gauge, ok := m.Data.(metricdata.Gauge[int64])
	require.True(t, ok, "%s is not an int64 gauge", m.Name)
	for _, dp := range gauge.DataPoints {
		if hasAttributes(dp.Attributes, want) {
			return dp.Value, true
		}
	}
	return 0, false
}

func histogramCount(t *testing.T, m metricdata.Metrics, want ...attribute.KeyValue) uint64 {
	var total uint64
	switch h := m.Data.(type) {
	case metricdata.Histogram[int64]:
		for _, dp := range h.DataPoints {
			if hasAttributes(dp.Attributes, want) {
				total += dp.Count
			}
		}
	case metricdata.Histogram[float64]:
		for _, dp := range h.DataPoints {
			if hasAttributes(dp.Attributes, want) {
				total += dp.Count
			}
		}
	default:
		require.Failf(t, "wrong type", "%s is not a histogram", m.Name)
	}
	return total
}

func makeEventsClient(t *testing.T, hook ldhooks.Hook, eventsURL string, events *ldcomponents.EventProcessorBuilder,
) *ldclient.LDClient {
	client, err := ldclient.MakeCustomClient("sdk-key", ldclient.Config{
		DataSource:       ldtestdata.DataSource(),
		Events:           events,
		ServiceEndpoints: interfaces.ServiceEndpoints{Events: eventsURL},
		Hooks:            []ldhooks.Hook{hook},
		// No diagnostic init event: it would occupy the flush channel at startup and a flush
		// requested while the channel is full is skipped without completing FlushAndWait.
		DiagnosticOptOut: true,
	}, time.Second)
	require.NoError(t, err)
	return client
}

// flushAndWait flushes until the event processor confirms delivery or failure. A flush that
// arrives while a previous payload is still waiting for a worker is skipped by the event
// processor, so a single FlushAndWait can time out even though nothing is wrong.
func flushAndWait(t *testing.T, client *ldclient.LDClient, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if client.FlushAndWait(timeout) {
			return
		}
	}
	require.Fail(t, "events were not flushed within the timeout")
}

func TestMetricsHookRecordsDeliveredEvents(t *testing.T) {
	setup := newMetricsTestSetup(t)
	handler, requests := httphelpers.RecordingHandler(httphelpers.HandlerWithStatus(http.StatusAccepted))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := makeEventsClient(t, setup.hook, server.URL, ldcomponents.SendEvents())
	defer client.Close()

	require.NoError(t, client.Identify(ldcontext.New("user-a")))
	require.NoError(t, client.Identify(ldcontext.New("user-b")))
	flushAndWait(t, client, 2*time.Second)
	<-requests

	rm := setup.collect(t)
	assert.Equal(t, int64(2), sumInt64(t, requireMetric(t, rm, metricEventsDelivered)))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricEventsFlushes),
		attribute.String(attrFlushOutcome, flushOutcomeSuccess)))
	assert.Greater(t, sumInt64(t, requireMetric(t, rm, metricEventsSentSize)), int64(0))
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricEventsBatchSize),
		attribute.String(attrFlushOutcome, flushOutcomeSuccess)))
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricEventsFlushDuration),
		attribute.String(attrFlushOutcome, flushOutcomeSuccess)))
	assert.False(t, hasMetric(rm, metricEventsFailed))
	assert.False(t, hasMetric(rm, metricEventsDropped))
}

func TestMetricsHookRecordsFailedEvents(t *testing.T) {
	setup := newMetricsTestSetup(t)
	server := httptest.NewServer(httphelpers.HandlerWithStatus(http.StatusServiceUnavailable))
	defer server.Close()

	client := makeEventsClient(t, setup.hook, server.URL, ldcomponents.SendEvents())
	defer client.Close()

	require.NoError(t, client.Identify(ldcontext.New("user-a")))
	require.NoError(t, client.Identify(ldcontext.New("user-b")))
	require.NoError(t, client.Identify(ldcontext.New("user-c")))
	// The event sender retries a 503 once after a one second delay before giving up.
	flushAndWait(t, client, 5*time.Second)

	rm := setup.collect(t)
	failedAttrs := []attribute.KeyValue{
		attribute.String(attrErrorType, "503"),
		attribute.Int(attrHTTPStatusCode, http.StatusServiceUnavailable),
	}
	assert.Equal(t, int64(3), sumInt64(t, requireMetric(t, rm, metricEventsFailed), failedAttrs...))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricEventsFlushes),
		append(failedAttrs, attribute.String(attrFlushOutcome, flushOutcomeFailure))...))
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricEventsBatchSize),
		attribute.String(attrFlushOutcome, flushOutcomeFailure)))
	assert.False(t, hasMetric(rm, metricEventsDelivered))
}

func TestMetricsHookRecordsFailedEventsWithoutResponse(t *testing.T) {
	setup := newMetricsTestSetup(t)
	server := httptest.NewServer(httphelpers.BrokenConnectionHandler())
	defer server.Close()

	client := makeEventsClient(t, setup.hook, server.URL, ldcomponents.SendEvents())
	defer client.Close()

	require.NoError(t, client.Identify(ldcontext.New("user-a")))
	flushAndWait(t, client, 5*time.Second)

	rm := setup.collect(t)
	failed := requireMetric(t, rm, metricEventsFailed)
	assert.Equal(t, int64(1), sumInt64(t, failed, attribute.String(attrErrorType, errorTypeOther)))
	sum := failed.Data.(metricdata.Sum[int64])
	for _, dp := range sum.DataPoints {
		_, hasStatus := dp.Attributes.Value(attrHTTPStatusCode)
		assert.False(t, hasStatus, "no status code should be reported for a network error")
	}
}

func TestMetricsHookRecordsDroppedEvents(t *testing.T) {
	setup := newMetricsTestSetup(t)
	server := httptest.NewServer(httphelpers.HandlerWithStatus(http.StatusAccepted))
	defer server.Close()

	client := makeEventsClient(t, setup.hook, server.URL, ldcomponents.SendEvents().Capacity(1))
	defer client.Close()

	for i := 0; i < 5; i++ {
		require.NoError(t, client.Identify(ldcontext.New("user-"+string(rune('a'+i)))))
	}
	flushAndWait(t, client, 2*time.Second)

	rm := setup.collect(t)
	// One event fits in the buffer; the other four are discarded either because the buffer is
	// full or because the dispatcher could not keep up. Both reasons count as dropped.
	dropped := requireMetric(t, rm, metricEventsDropped)
	capacityDrops := sumInt64(t, dropped, attribute.String(attrDropReason, string(ldhooks.EventsDroppedReasonCapacity)))
	backpressureDrops := sumInt64(t, dropped,
		attribute.String(attrDropReason, string(ldhooks.EventsDroppedReasonBackpressure)))
	assert.Equal(t, int64(4), capacityDrops+backpressureDrops)
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricEventsDelivered)))
}

func TestMetricsHookRecordsDataSourceStatus(t *testing.T) {
	setup := newMetricsTestSetup(t)

	flag := ldbuilders.NewFlagBuilder("flag").SingleVariation(ldvalue.Bool(true)).Build()
	putEvent := ldservices.NewServerSDKData().Flags(&flag).ToPutEvent()
	firstStream, firstControl := httphelpers.SSEHandler(&putEvent)
	secondStream, _ := httphelpers.SSEHandler(&putEvent)
	// Connection sequence: stream, then a 503, then a stream again. This produces one
	// VALID -> INTERRUPTED -> VALID cycle with a single error response.
	handler := httphelpers.HandlerForPath("/all",
		httphelpers.SequentialHandler(firstStream, httphelpers.HandlerWithStatus(http.StatusServiceUnavailable), secondStream),
		nil)
	server := httptest.NewServer(handler)
	defer server.Close()

	client, err := ldclient.MakeCustomClient("sdk-key", ldclient.Config{
		DataSource:       ldcomponents.StreamingDataSource().InitialReconnectDelay(10 * time.Millisecond),
		Events:           ldcomponents.NoEvents(),
		ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL},
		Hooks:            []ldhooks.Hook{setup.hook},
	}, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	statusProvider := client.GetDataSourceStatusProvider()
	require.Equal(t, interfaces.DataSourceStateValid, statusProvider.GetStatus().State)

	// Drop the stream; the reconnect gets the 503 and the next reconnect succeeds.
	firstControl.EndAll()
	require.True(t, statusProvider.WaitFor(interfaces.DataSourceStateInterrupted, 5*time.Second))
	require.True(t, statusProvider.WaitFor(interfaces.DataSourceStateValid, 5*time.Second))

	rm := setup.collect(t)

	errors := requireMetric(t, rm, metricDataSourceErrors)
	assert.Equal(t, int64(1), sumInt64(t, errors,
		attribute.String(attrDataSourceErrorKind, string(interfaces.DataSourceErrorKindErrorResponse)),
		attribute.Int(attrHTTPStatusCode, http.StatusServiceUnavailable),
		attribute.String(attrErrorType, "503"),
	))

	transitions := requireMetric(t, rm, metricDataSourceTransitions)
	assert.Equal(t, int64(1), sumInt64(t, transitions,
		stateAttribute(interfaces.DataSourceStateInterrupted),
		attribute.String(attrPreviousState, string(interfaces.DataSourceStateValid))))
	assert.Equal(t, int64(1), sumInt64(t, transitions,
		stateAttribute(interfaces.DataSourceStateValid),
		attribute.String(attrPreviousState, string(interfaces.DataSourceStateInterrupted))))

	state := requireMetric(t, rm, metricDataSourceState)
	valid, ok := lastGaugeInt64(t, state, stateAttribute(interfaces.DataSourceStateValid))
	require.True(t, ok)
	assert.Equal(t, int64(1), valid)
	interrupted, ok := lastGaugeInt64(t, state, stateAttribute(interfaces.DataSourceStateInterrupted))
	require.True(t, ok)
	assert.Equal(t, int64(0), interrupted)
	off, ok := lastGaugeInt64(t, state, stateAttribute(interfaces.DataSourceStateOff))
	require.True(t, ok)
	assert.Equal(t, int64(0), off)

	assert.Greater(t, sumFloat64(t, requireMetric(t, rm, metricDataSourceInterrupted)), 0.0)
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricDataSourceInterruption)))

	// The state duration gauge reports the time in the current state, and 0 for every other state.
	age := requireMetric(t, rm, metricDataSourceStateAge)
	gauge, ok := age.Data.(metricdata.Gauge[float64])
	require.True(t, ok)
	require.Len(t, gauge.DataPoints, 4)
	for _, dp := range gauge.DataPoints {
		if hasAttributes(dp.Attributes, []attribute.KeyValue{stateAttribute(interfaces.DataSourceStateValid)}) {
			assert.Greater(t, dp.Value, 0.0)
		} else {
			assert.Equal(t, 0.0, dp.Value)
		}
	}
}

func TestMetricsHookMetadata(t *testing.T) {
	setup := newMetricsTestSetup(t)
	assert.Equal(t, "LaunchDarkly Metrics Hook", setup.hook.Metadata().Name())
}
