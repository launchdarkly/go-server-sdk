package ldotel

import (
	gocontext "context"
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// These tests drive the handlers directly, so that the order of the invocations and the times in
// the statuses are exact.

var (
	streamingDescriptor = interfaces.DataSourceDescriptor{
		Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportStreaming, Name: "streaming",
	}
	pollingDescriptor = interfaces.DataSourceDescriptor{
		Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportPolling, Name: "polling",
	}
	relayPollDescriptor = interfaces.DataSourceDescriptor{
		Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportPolling, Name: "relay-poll",
	}
)

func statusAt(state interfaces.DataSourceState, since time.Time) interfaces.DataSourceStatus {
	return interfaces.DataSourceStatus{State: state, StateSince: since}
}

func reportStatus(t *testing.T, hook *MetricsHook, previous, current interfaces.DataSourceStatus) {
	require.NoError(t, hook.DataSourceStatusChanged(gocontext.Background(),
		ldhooks.NewDataSourceStatusContext(previous, current)))
}

func reportSynchronizer(t *testing.T, hook *MetricsHook, previous, current interfaces.DataSourceDescriptor,
	reason ldhooks.SynchronizerChangeReason) {
	require.NoError(t, hook.SynchronizerChanged(gocontext.Background(),
		ldhooks.NewSynchronizerChangeContext(previous, current, reason, interfaces.DataSourceErrorInfo{})))
}

func withState(d interfaces.DataSourceDescriptor, state interfaces.DataSourceState) []attribute.KeyValue {
	return append(sourceAttributes(d), stateAttribute(state))
}

func lastGaugeFloat64(t *testing.T, m metricdata.Metrics, want ...attribute.KeyValue) (float64, bool) {
	gauge, ok := m.Data.(metricdata.Gauge[float64])
	require.True(t, ok, "%s is not a float64 gauge", m.Name)
	for _, dp := range gauge.DataPoints {
		if hasAttributes(dp.Attributes, want) {
			return dp.Value, true
		}
	}
	return 0, false
}

func requireGaugeInt64(t *testing.T, rm metricdata.ResourceMetrics, name string, want ...attribute.KeyValue) int64 {
	value, ok := lastGaugeInt64(t, requireMetric(t, rm, name), want...)
	require.True(t, ok, "no %s series with attributes %v", name, want)
	return value
}

func TestMetricsHookAttributesStatusesBeforeAnyComponentToTheDataSystem(t *testing.T) {
	setup := newMetricsTestSetup(t)
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{},
		statusAt(interfaces.DataSourceStateInitializing, time.Now()))

	rm := setup.collect(t)
	systemInitializing := []attribute.KeyValue{
		attribute.String(attrDataSourceName, dataSystemName),
		attribute.String(attrDataSourceProtocol, notProvidedValue),
		attribute.String(attrDataSourceTransport, notProvidedValue),
		stateAttribute(interfaces.DataSourceStateInitializing),
	}
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState, systemInitializing...))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceTransitions),
		append(systemInitializing, attribute.String(attrPreviousState, noneValue))...))

	// One component, four states.
	state := requireMetric(t, rm, metricDataSourceState).Data.(metricdata.Gauge[int64])
	assert.Len(t, state.DataPoints, 4)
}

func TestMetricsHookMovesTheStateToAnAppliedInitializerBeforeTheNextStatus(t *testing.T) {
	setup := newMetricsTestSetup(t)
	ctx := gocontext.Background()
	initializing := statusAt(interfaces.DataSourceStateInitializing, time.Now())
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, initializing)
	require.NoError(t, setup.hook.InitializerCompleted(ctx, ldhooks.NewInitializerContext(
		relayPollDescriptor, ldhooks.InitializerOutcomeFailed, errors.New("connection refused"), 10*time.Millisecond, false)))
	require.NoError(t, setup.hook.InitializerCompleted(ctx, ldhooks.NewInitializerContext(
		pollingDescriptor, ldhooks.InitializerOutcomeSucceeded, nil, 20*time.Millisecond, true)))

	// Between the applied initializer and the next status, the initializer already reports the
	// state that carried over. The failed initializer never reports a state.
	rm := setup.collect(t)
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(pollingDescriptor, interfaces.DataSourceStateInitializing)...))
	assert.Equal(t, int64(0), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(dataSystemDescriptor, interfaces.DataSourceStateInitializing)...))
	_, ok := lastGaugeInt64(t, requireMetric(t, rm, metricDataSourceState), sourceAttributes(relayPollDescriptor)...)
	assert.False(t, ok, "a failed initializer has no state series")

	attempts := requireMetric(t, rm, metricInitializerAttempts)
	assert.Equal(t, int64(1), sumInt64(t, attempts, append(sourceAttributes(relayPollDescriptor),
		attribute.String(attrInitializerOutcome, string(ldhooks.InitializerOutcomeFailed)))...))
	assert.Equal(t, int64(1), sumInt64(t, attempts, append(sourceAttributes(pollingDescriptor),
		attribute.String(attrInitializerOutcome, string(ldhooks.InitializerOutcomeSucceeded)))...))

	reportStatus(t, setup.hook, initializing, statusAt(interfaces.DataSourceStateValid, time.Now()))
	rm = setup.collect(t)
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(pollingDescriptor, interfaces.DataSourceStateValid)...))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceTransitions),
		append(withState(pollingDescriptor, interfaces.DataSourceStateValid),
			attribute.String(attrPreviousState, string(interfaces.DataSourceStateInitializing)))...))
}

func TestMetricsHookChargesAnInterruptionToTheComponentThatFailed(t *testing.T) {
	setup := newMetricsTestSetup(t)
	start := time.Now().Add(-time.Minute)
	reportSynchronizer(t, setup.hook, interfaces.DataSourceDescriptor{}, streamingDescriptor,
		ldhooks.SynchronizerChangeReasonInitial)
	initializing := statusAt(interfaces.DataSourceStateInitializing, start)
	valid := statusAt(interfaces.DataSourceStateValid, start.Add(time.Second))
	interrupted := statusAt(interfaces.DataSourceStateInterrupted, start.Add(10*time.Second))
	interrupted.LastError = interfaces.DataSourceErrorInfo{
		Kind: interfaces.DataSourceErrorKindNetworkError, Time: interrupted.StateSince,
	}
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, initializing)
	reportStatus(t, setup.hook, initializing, valid)
	reportStatus(t, setup.hook, valid, interrupted)

	// The polling synchronizer takes over while the data source is interrupted.
	reportSynchronizer(t, setup.hook, streamingDescriptor, pollingDescriptor, ldhooks.SynchronizerChangeReasonFallback)
	recovered := statusAt(interfaces.DataSourceStateValid, start.Add(40*time.Second))
	recovered.LastError = interrupted.LastError
	reportStatus(t, setup.hook, interrupted, recovered)

	rm := setup.collect(t)
	interruptedTime := requireMetric(t, rm, metricDataSourceInterrupted)
	assert.InDelta(t, 30.0, sumFloat64(t, interruptedTime, sourceAttributes(streamingDescriptor)...), 0.001)
	assert.Equal(t, 0.0, sumFloat64(t, interruptedTime, sourceAttributes(pollingDescriptor)...))
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricDataSourceInterruption),
		sourceAttributes(streamingDescriptor)...))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceErrors),
		sourceAttributes(streamingDescriptor)...))

	// The recovery itself is reported by the component that took over.
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceTransitions),
		append(withState(pollingDescriptor, interfaces.DataSourceStateValid),
			attribute.String(attrPreviousState, string(interfaces.DataSourceStateInterrupted)))...))
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(pollingDescriptor, interfaces.DataSourceStateValid)...))
	assert.Equal(t, int64(0), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(streamingDescriptor, interfaces.DataSourceStateValid)...))
	assert.Equal(t, int64(0), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(streamingDescriptor, interfaces.DataSourceStateInterrupted)...))
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricSynchronizerActive, sourceAttributes(pollingDescriptor)...))
	assert.Equal(t, int64(0), requireGaugeInt64(t, rm, metricSynchronizerActive, sourceAttributes(streamingDescriptor)...))

	// The time in VALID counts from the takeover, not from the earlier stateSince.
	age, ok := lastGaugeFloat64(t, requireMetric(t, rm, metricDataSourceStateAge),
		withState(pollingDescriptor, interfaces.DataSourceStateValid)...)
	require.True(t, ok)
	assert.Less(t, age, 5.0)
}

func TestMetricsHookKeepsTheComponentWhenSynchronizersAreExhausted(t *testing.T) {
	setup := newMetricsTestSetup(t)
	reportSynchronizer(t, setup.hook, interfaces.DataSourceDescriptor{}, streamingDescriptor,
		ldhooks.SynchronizerChangeReasonInitial)
	initializing := statusAt(interfaces.DataSourceStateInitializing, time.Now())
	valid := statusAt(interfaces.DataSourceStateValid, time.Now())
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, initializing)
	reportStatus(t, setup.hook, initializing, valid)
	reportSynchronizer(t, setup.hook, streamingDescriptor, interfaces.DataSourceDescriptor{},
		ldhooks.SynchronizerChangeReasonExhausted)
	reportStatus(t, setup.hook, valid, statusAt(interfaces.DataSourceStateOff, time.Now()))

	rm := setup.collect(t)
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		withState(streamingDescriptor, interfaces.DataSourceStateOff)...))
	assert.Equal(t, int64(0), requireGaugeInt64(t, rm, metricSynchronizerActive, sourceAttributes(streamingDescriptor)...))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricSynchronizerTransitions),
		attribute.String(attrSynchronizerPrevious, "streaming"),
		attribute.String(attrSynchronizerCurrent, noneValue),
		attribute.String(attrSynchronizerReason, string(ldhooks.SynchronizerChangeReasonExhausted))))
}

func TestMetricsHookReportsAnUnnamedComponentAsNotProvided(t *testing.T) {
	setup := newMetricsTestSetup(t)
	unnamed := interfaces.DataSourceDescriptor{
		Protocol: interfaces.DataSourceProtocolFDv2, Transport: interfaces.DataSourceTransportPolling,
	}
	reportSynchronizer(t, setup.hook, interfaces.DataSourceDescriptor{}, unnamed, ldhooks.SynchronizerChangeReasonInitial)
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, statusAt(interfaces.DataSourceStateValid, time.Now()))

	rm := setup.collect(t)
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricSynchronizerTransitions),
		attribute.String(attrSynchronizerPrevious, noneValue),
		attribute.String(attrSynchronizerCurrent, notProvidedValue)))
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		attribute.String(attrDataSourceName, notProvidedValue),
		attribute.String(attrDataSourceProtocol, "fdv2"),
		attribute.String(attrDataSourceTransport, "polling"),
		stateAttribute(interfaces.DataSourceStateValid)))
}

func TestMetricsHookClampsANegativeInterruptionToZero(t *testing.T) {
	setup := newMetricsTestSetup(t)
	now := time.Now()
	reportSynchronizer(t, setup.hook, interfaces.DataSourceDescriptor{}, streamingDescriptor,
		ldhooks.SynchronizerChangeReasonInitial)
	valid := statusAt(interfaces.DataSourceStateValid, now)
	// The clock stepped back: the recovery is stamped earlier than the interruption.
	interrupted := statusAt(interfaces.DataSourceStateInterrupted, now.Add(10*time.Second))
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, valid)
	reportStatus(t, setup.hook, valid, interrupted)
	reportStatus(t, setup.hook, interrupted, statusAt(interfaces.DataSourceStateValid, now.Add(5*time.Second)))

	rm := setup.collect(t)
	assert.Equal(t, 0.0, sumFloat64(t, requireMetric(t, rm, metricDataSourceInterrupted)))
	assert.Equal(t, uint64(1), histogramCount(t, requireMetric(t, rm, metricDataSourceInterruption)))
}

func TestMetricsHookStopsObservingWhenClosed(t *testing.T) {
	setup := newMetricsTestSetup(t)
	reportSynchronizer(t, setup.hook, interfaces.DataSourceDescriptor{}, streamingDescriptor,
		ldhooks.SynchronizerChangeReasonInitial)
	reportStatus(t, setup.hook, interfaces.DataSourceStatus{}, statusAt(interfaces.DataSourceStateValid, time.Now()))
	rm := setup.collect(t)
	require.True(t, hasMetric(rm, metricDataSourceState))
	require.True(t, hasMetric(rm, metricSynchronizerActive))

	require.NoError(t, setup.hook.Close())
	rm = setup.collect(t)
	assert.False(t, hasMetric(rm, metricDataSourceState))
	assert.False(t, hasMetric(rm, metricDataSourceStateAge))
	assert.False(t, hasMetric(rm, metricSynchronizerActive))
	// The counters keep what they recorded.
	assert.True(t, hasMetric(rm, metricSynchronizerTransitions))

	require.NoError(t, setup.hook.Close(), "a second Close is harmless")
}

func TestMetricsHookAddsConfiguredAttributes(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	clientName := attribute.String("launchdarkly.client", "checkout")
	hook, err := NewMetricsHook(WithMeterProvider(provider), WithAttributes(clientName))
	require.NoError(t, err)
	setup := &metricsTestSetup{reader: reader, hook: hook}

	reportStatus(t, hook, interfaces.DataSourceStatus{}, statusAt(interfaces.DataSourceStateValid, time.Now()))
	require.NoError(t, hook.EventFlushCompleted(gocontext.Background(),
		ldhooks.NewEventFlushContext(3, 100, true, 202, 10*time.Millisecond, 0)))

	rm := setup.collect(t)
	assert.Equal(t, int64(1), requireGaugeInt64(t, rm, metricDataSourceState,
		clientName, stateAttribute(interfaces.DataSourceStateValid)))
	assert.Equal(t, int64(3), sumInt64(t, requireMetric(t, rm, metricEventsSent), clientName))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricEventsFlushes), clientName,
		attribute.String(attrFlushOutcome, flushOutcomeSucceeded)))
	assert.Equal(t, int64(1), sumInt64(t, requireMetric(t, rm, metricDataSourceTransitions), clientName))
}

func TestMetricsHookUsesTheGlobalMeterProviderByDefault(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(provider)
	t.Cleanup(func() { otel.SetMeterProvider(noop.NewMeterProvider()) })

	hook, err := NewMetricsHook()
	require.NoError(t, err)
	require.NoError(t, hook.EventFlushCompleted(gocontext.Background(),
		ldhooks.NewEventFlushContext(2, 50, true, 202, time.Millisecond, 0)))

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(gocontext.Background(), &rm))
	assert.Equal(t, int64(2), sumInt64(t, requireMetric(t, rm, metricEventsSent)))
}
