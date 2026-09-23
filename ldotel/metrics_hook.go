package ldotel

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// meterName is the instrumentation scope. It matches the tracer name used by TracingHook.
const meterName = "launchdarkly-client"

// Instrument names.
const (
	metricEventsSent              = "launchdarkly.sdk.events.sent"
	metricEventsFailed            = "launchdarkly.sdk.events.failed"
	metricEventsDropped           = "launchdarkly.sdk.events.dropped"
	metricEventsFlushes           = "launchdarkly.sdk.events.flushes"
	metricEventsBatchSize         = "launchdarkly.sdk.events.batch.size"
	metricEventsFlushDuration     = "launchdarkly.sdk.events.flush.duration"
	metricEventsSentSize          = "launchdarkly.sdk.events.sent.size"
	metricDataSourceErrors        = "launchdarkly.sdk.data_source.errors"
	metricDataSourceState         = "launchdarkly.sdk.data_source.state"
	metricDataSourceTransitions   = "launchdarkly.sdk.data_source.state.transitions"
	metricDataSourceStateAge      = "launchdarkly.sdk.data_source.state.duration"
	metricDataSourceInterrupted   = "launchdarkly.sdk.data_source.interrupted.time"
	metricDataSourceInterruption  = "launchdarkly.sdk.data_source.interruption.duration"
	metricInitializerAttempts     = "launchdarkly.sdk.data_source.initializer.attempts"
	metricInitializerDuration     = "launchdarkly.sdk.data_source.initializer.duration"
	metricInitializationDuration  = "launchdarkly.sdk.data_source.initialization.duration"
	metricSynchronizerTransitions = "launchdarkly.sdk.data_source.synchronizer.transitions"
	metricSynchronizerActive      = "launchdarkly.sdk.data_source.synchronizer.active"
)

// Attribute keys. error.type and http.response.status_code follow the OpenTelemetry semantic
// conventions; the others are LaunchDarkly specific.
const (
	attrFlushOutcome          = "launchdarkly.sdk.events.flush.outcome"
	attrDataSourceState       = "launchdarkly.sdk.data_source.state"
	attrPreviousState         = "launchdarkly.sdk.data_source.state.previous"
	attrDataSourceErrorKind   = "launchdarkly.sdk.data_source.error.kind"
	attrDataSourceName        = "launchdarkly.sdk.data_source.name"
	attrDataSourceProtocol    = "launchdarkly.sdk.data_source.protocol"
	attrDataSourceTransport   = "launchdarkly.sdk.data_source.transport"
	attrInitializerOutcome    = "launchdarkly.sdk.data_source.initializer.outcome"
	attrInitializationOutcome = "launchdarkly.sdk.data_source.initialization.outcome"
	attrSynchronizerPrevious  = "launchdarkly.sdk.data_source.synchronizer.previous"
	attrSynchronizerCurrent   = "launchdarkly.sdk.data_source.synchronizer.current"
	attrSynchronizerReason    = "launchdarkly.sdk.data_source.synchronizer.reason"
	attrErrorType             = "error.type"
	attrHTTPStatusCode        = "http.response.status_code"
)

const (
	flushOutcomeSuccess = "success"
	flushOutcomeFailure = "failure"
	// errorTypeOther is the semantic-convention value for an error that has no more specific type.
	errorTypeOther = "_OTHER"
	// stateNone is reported as the previous state for the first status the SDK reports.
	stateNone = "NONE"
	// unknownValue is reported for a descriptor field the component did not provide.
	unknownValue = "not_provided"
	// noSynchronizer is reported when no synchronizer is on either side of a change.
	noSynchronizer          = "none"
	initializationSucceeded = "succeeded"
	initializationFailed    = "failed"
)

// MetricsHookOption is used to implement functional options for the MetricsHook.
type MetricsHookOption func(hook *MetricsHook)

// WithMeterProvider sets the MeterProvider used to create instruments. By default the hook uses
// the global provider returned by otel.GetMeterProvider.
func WithMeterProvider(provider metric.MeterProvider) MetricsHookOption {
	return func(h *MetricsHook) {
		h.meterProvider = provider
	}
}

// A MetricsHook reports SDK operational metrics through the OpenTelemetry metrics API.
//
// The hook records analytics event delivery (events delivered, failed, and dropped; flush counts,
// batch sizes, and durations) and data source health (state, state transitions, errors, time spent
// interrupted, initializer outcomes, and the active synchronizer). It does not record anything per
// evaluation; use TracingHook for evaluation telemetry.
//
// Data source statuses are attributed to the component that reports them. The hook learns the
// component from the initializer and synchronizer handlers, not from the status itself.
//
// All instruments are created when the hook is constructed. The hook is safe for concurrent use.
type MetricsHook struct {
	ldhooks.Unimplemented
	metadata      ldhooks.Metadata
	meterProvider metric.MeterProvider

	eventsSent              metric.Int64Counter
	eventsFailed            metric.Int64Counter
	eventsDropped           metric.Int64Counter
	eventsFlushes           metric.Int64Counter
	eventsBatchSize         metric.Int64Histogram
	eventsFlushDuration     metric.Float64Histogram
	eventsSentSize          metric.Int64Counter
	dataSourceErrors        metric.Int64Counter
	dataSourceState         metric.Int64Gauge
	dataSourceTransitions   metric.Int64Counter
	dataSourceInterrupted   metric.Float64Counter
	dataSourceInterruption  metric.Float64Histogram
	initializerAttempts     metric.Int64Counter
	initializerDuration     metric.Float64Histogram
	initializationDuration  metric.Float64Histogram
	synchronizerTransitions metric.Int64Counter
	synchronizerActive      metric.Int64Gauge

	// Pre-built attribute options for the hot paths.
	successAttrs metric.MeasurementOption
	failureAttrs metric.MeasurementOption

	mu         sync.Mutex
	lastStatus interfaces.DataSourceStatus
	haveStatus bool
	// currentSource is the component that reports statuses now, from the lifecycle handlers.
	currentSource interfaces.DataSourceDescriptor
	// attributedSource is the component the state gauges currently describe.
	attributedSource interfaces.DataSourceDescriptor
	// seenSources lists every component that has been attributed a status. The state gauges write
	// a value for each of them so that a component that stopped reporting reads 0, not a stale value.
	seenSources []interfaces.DataSourceDescriptor
	// seenSynchronizers lists every synchronizer that has started, for the same reason.
	seenSynchronizers []interfaces.DataSourceDescriptor
}

// allDataSourceStates lists every state the SDK can report. The per-state gauges write a value for
// each of them on every change (1 or the duration for the current state, 0 for the others). Writing
// all four keeps the series continuous and overwrites a stale value left behind by a previous
// process that exported under the same instance identity.
var allDataSourceStates = []interfaces.DataSourceState{ //nolint:gochecknoglobals
	interfaces.DataSourceStateInitializing,
	interfaces.DataSourceStateValid,
	interfaces.DataSourceStateInterrupted,
	interfaces.DataSourceStateOff,
}

// Metadata returns meta-data about the metrics hook.
func (h *MetricsHook) Metadata() ldhooks.Metadata {
	return h.metadata
}

// instrumentBuilder creates instruments and keeps the first error.
type instrumentBuilder struct {
	meter metric.Meter
	err   error
}

func (b *instrumentBuilder) int64Counter(name, desc, unit string) metric.Int64Counter {
	c, err := b.meter.Int64Counter(name, metric.WithDescription(desc), metric.WithUnit(unit))
	b.keep(err)
	return c
}

func (b *instrumentBuilder) float64Counter(name, desc, unit string) metric.Float64Counter {
	c, err := b.meter.Float64Counter(name, metric.WithDescription(desc), metric.WithUnit(unit))
	b.keep(err)
	return c
}

func (b *instrumentBuilder) int64Gauge(name, desc, unit string) metric.Int64Gauge {
	g, err := b.meter.Int64Gauge(name, metric.WithDescription(desc), metric.WithUnit(unit))
	b.keep(err)
	return g
}

func (b *instrumentBuilder) int64Histogram(name, desc, unit string, buckets ...float64) metric.Int64Histogram {
	hg, err := b.meter.Int64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit),
		metric.WithExplicitBucketBoundaries(buckets...))
	b.keep(err)
	return hg
}

func (b *instrumentBuilder) float64Histogram(name, desc, unit string, buckets ...float64) metric.Float64Histogram {
	hg, err := b.meter.Float64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit),
		metric.WithExplicitBucketBoundaries(buckets...))
	b.keep(err)
	return hg
}

func (b *instrumentBuilder) keep(err error) {
	if b.err == nil && err != nil {
		b.err = err
	}
}

// NewMetricsHook creates a new MetricsHook instance. The MetricsHook can be provided to the
// LaunchDarkly client in order to report SDK operational metrics through OpenTelemetry.
func NewMetricsHook(opts ...MetricsHookOption) (*MetricsHook, error) {
	h := &MetricsHook{
		metadata: ldhooks.NewMetadata("LaunchDarkly Metrics Hook"),
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.meterProvider == nil {
		h.meterProvider = otel.GetMeterProvider()
	}
	b := &instrumentBuilder{meter: h.meterProvider.Meter(meterName)}

	h.eventsSent = b.int64Counter(metricEventsSent,
		"Analytics events accepted by the LaunchDarkly events service", "{event}")
	h.eventsFailed = b.int64Counter(metricEventsFailed,
		"Analytics events lost because a batch could not be delivered after all retries", "{event}")
	h.eventsDropped = b.int64Counter(metricEventsDropped,
		"Analytics events discarded before delivery because the SDK event buffer was full, reported with the next flush", "{event}")
	h.eventsFlushes = b.int64Counter(metricEventsFlushes,
		"Attempts to deliver a batch of analytics events", "{flush}")
	h.eventsBatchSize = b.int64Histogram(metricEventsBatchSize,
		"Number of analytics events in each delivered or failed batch", "{event}",
		1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000)
	h.eventsFlushDuration = b.float64Histogram(metricEventsFlushDuration,
		"Time taken to deliver a batch of analytics events, including any retry", "s",
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10)
	h.eventsSentSize = b.int64Counter(metricEventsSentSize,
		"Bytes of analytics event payloads accepted by the LaunchDarkly events service, before compression", "By")
	h.dataSourceErrors = b.int64Counter(metricDataSourceErrors,
		"Errors reported by the data source while connecting to or reading from LaunchDarkly", "{error}")
	h.dataSourceState = b.int64Gauge(metricDataSourceState,
		"1 for the current data source state, 0 for every other state", "{state}")
	h.dataSourceTransitions = b.int64Counter(metricDataSourceTransitions,
		"Data source state changes", "{transition}")
	h.dataSourceInterrupted = b.float64Counter(metricDataSourceInterrupted,
		"Cumulative time the data source has spent in the INTERRUPTED state", "s")
	h.dataSourceInterruption = b.float64Histogram(metricDataSourceInterruption,
		"Duration of each period spent in the INTERRUPTED state", "s",
		1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600)
	h.initializerAttempts = b.int64Counter(metricInitializerAttempts,
		"Attempts to obtain initial flag data from a data initializer, by outcome", "{attempt}")
	h.initializerDuration = b.float64Histogram(metricInitializerDuration,
		"Time taken by each data initializer attempt", "s",
		0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60)
	h.initializationDuration = b.float64Histogram(metricInitializationDuration,
		"Time from data system start until the SDK first had data or gave up", "s",
		0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120)
	h.synchronizerTransitions = b.int64Counter(metricSynchronizerTransitions,
		"Changes of the active data synchronizer, by reason", "{transition}")
	h.synchronizerActive = b.int64Gauge(metricSynchronizerActive,
		"1 for the active data synchronizer, 0 for every other synchronizer that has run", "{synchronizer}")
	_, err := b.meter.Float64ObservableGauge(metricDataSourceStateAge,
		metric.WithDescription("Time the data source has spent in its current state"),
		metric.WithUnit("s"),
		metric.WithFloat64Callback(h.observeStateDuration))
	b.keep(err)
	if b.err != nil {
		return nil, b.err
	}

	h.successAttrs = metric.WithAttributes(attribute.String(attrFlushOutcome, flushOutcomeSuccess))
	h.failureAttrs = metric.WithAttributes(attribute.String(attrFlushOutcome, flushOutcomeFailure))
	return h, nil
}

// sourceAttributes returns the attributes that identify a data source component. Fields the
// component did not provide are reported as "unknown" so that series carry a consistent key set.
func sourceAttributes(d interfaces.DataSourceDescriptor) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(attrDataSourceName, orUnknown(d.Name)),
		attribute.String(attrDataSourceProtocol, orUnknown(string(d.Protocol))),
		attribute.String(attrDataSourceTransport, orUnknown(string(d.Transport))),
	}
}

func orUnknown(value string) string {
	if value == "" {
		return unknownValue
	}
	return value
}

func withSource(d interfaces.DataSourceDescriptor, extra ...attribute.KeyValue) metric.MeasurementOption {
	return metric.WithAttributes(append(sourceAttributes(d), extra...)...)
}

func containsDescriptor(list []interfaces.DataSourceDescriptor, d interfaces.DataSourceDescriptor) bool {
	for _, item := range list {
		if item == d {
			return true
		}
	}
	return false
}

func (h *MetricsHook) observeStateDuration(_ context.Context, o metric.Float64Observer) error {
	h.mu.Lock()
	status, source, ok := h.lastStatus, h.attributedSource, h.haveStatus
	sources := append([]interfaces.DataSourceDescriptor(nil), h.seenSources...)
	h.mu.Unlock()
	if !ok || status.State == "" {
		return nil
	}
	for _, s := range sources {
		for _, state := range allDataSourceStates {
			seconds := 0.0
			if s == source && state == status.State {
				seconds = time.Since(status.StateSince).Seconds()
			}
			o.Observe(seconds, withSource(s, stateAttribute(state)))
		}
	}
	return nil
}

// writeStateGauge writes 1 for the given state and 0 for every other state, for one component.
func (h *MetricsHook) writeStateGauge(ctx context.Context, source interfaces.DataSourceDescriptor,
	current interfaces.DataSourceState) {
	for _, state := range allDataSourceStates {
		var value int64
		if state == current {
			value = 1
		}
		h.dataSourceState.Record(ctx, value, withSource(source, stateAttribute(state)))
	}
}

// clearStateGauge writes 0 for every state of a component that no longer reports statuses.
func (h *MetricsHook) clearStateGauge(ctx context.Context, source interfaces.DataSourceDescriptor) {
	for _, state := range allDataSourceStates {
		h.dataSourceState.Record(ctx, 0, withSource(source, stateAttribute(state)))
	}
}

// DataSourceStatusChanged implements the DataSourceStatusChanged handler.
func (h *MetricsHook) DataSourceStatusChanged(
	ctx context.Context,
	statusContext ldhooks.DataSourceStatusContext,
) error {
	previous, current := statusContext.Previous(), statusContext.Current()

	h.mu.Lock()
	source := h.currentSource
	previousSource, hadStatus := h.attributedSource, h.haveStatus
	h.lastStatus = current
	h.haveStatus = true
	h.attributedSource = source
	if !containsDescriptor(h.seenSources, source) {
		h.seenSources = append(h.seenSources, source)
	}
	h.mu.Unlock()

	sourceChanged := hadStatus && previousSource != source
	if current.State != previous.State || sourceChanged {
		if sourceChanged {
			h.clearStateGauge(ctx, previousSource)
		}
		h.writeStateGauge(ctx, source, current.State)
	}

	if current.State != previous.State {
		previousName := stateNone
		if previous.State != "" {
			previousName = string(previous.State)
		}
		h.dataSourceTransitions.Add(ctx, 1, withSource(source,
			stateAttribute(current.State),
			attribute.String(attrPreviousState, previousName),
		))
		if previous.State == interfaces.DataSourceStateInterrupted {
			seconds := current.StateSince.Sub(previous.StateSince).Seconds()
			if seconds < 0 {
				seconds = 0
			}
			h.dataSourceInterrupted.Add(ctx, seconds, withSource(source))
			h.dataSourceInterruption.Record(ctx, seconds, withSource(source))
		}
	}

	if current.LastError.Kind != "" && current.LastError != previous.LastError {
		h.dataSourceErrors.Add(ctx, 1, withSource(source, errorAttributes(current.LastError)...))
	}
	return nil
}

func stateAttribute(state interfaces.DataSourceState) attribute.KeyValue {
	return attribute.String(attrDataSourceState, string(state))
}

func errorAttributes(e interfaces.DataSourceErrorInfo) []attribute.KeyValue {
	attrs := []attribute.KeyValue{attribute.String(attrDataSourceErrorKind, string(e.Kind))}
	if e.StatusCode > 0 {
		attrs = append(attrs,
			attribute.Int(attrHTTPStatusCode, e.StatusCode),
			attribute.String(attrErrorType, strconv.Itoa(e.StatusCode)))
	} else {
		attrs = append(attrs, attribute.String(attrErrorType, string(e.Kind)))
	}
	return attrs
}

// InitializerCompleted implements the InitializerCompleted handler.
func (h *MetricsHook) InitializerCompleted(ctx context.Context, initializerContext ldhooks.InitializerContext) error {
	if initializerContext.Applied() {
		// Statuses reported during the initializer phase belong to the initializer whose data was applied.
		h.mu.Lock()
		h.currentSource = initializerContext.DataSource()
		h.mu.Unlock()
	}
	attrs := withSource(initializerContext.DataSource(),
		attribute.String(attrInitializerOutcome, string(initializerContext.Outcome())))
	h.initializerAttempts.Add(ctx, 1, attrs)
	h.initializerDuration.Record(ctx, initializerContext.Duration().Seconds(), attrs)
	return nil
}

// SynchronizerChanged implements the SynchronizerChanged handler.
func (h *MetricsHook) SynchronizerChanged(ctx context.Context, changeContext ldhooks.SynchronizerChangeContext) error {
	previous, current := changeContext.Previous(), changeContext.Current()

	h.mu.Lock()
	var reattributeFrom interfaces.DataSourceDescriptor
	reattribute := false
	var lastState interfaces.DataSourceState
	if current.IsDefined() {
		h.currentSource = current
		if !containsDescriptor(h.seenSynchronizers, current) {
			h.seenSynchronizers = append(h.seenSynchronizers, current)
		}
		// The new synchronizer reports the statuses that follow. Move the state gauge to it now so
		// that an unchanged status is not left attributed to the previous component.
		if h.haveStatus && h.attributedSource != current {
			reattribute = true
			reattributeFrom = h.attributedSource
			lastState = h.lastStatus.State
			h.attributedSource = current
			if !containsDescriptor(h.seenSources, current) {
				h.seenSources = append(h.seenSources, current)
			}
		}
	}
	seen := append([]interfaces.DataSourceDescriptor(nil), h.seenSynchronizers...)
	h.mu.Unlock()

	if reattribute {
		h.clearStateGauge(ctx, reattributeFrom)
		h.writeStateGauge(ctx, current, lastState)
	}

	previousName, currentName := noSynchronizer, noSynchronizer
	if previous.IsDefined() {
		previousName = orUnknown(previous.Name)
	}
	if current.IsDefined() {
		currentName = orUnknown(current.Name)
	}
	h.synchronizerTransitions.Add(ctx, 1, metric.WithAttributes(
		attribute.String(attrSynchronizerPrevious, previousName),
		attribute.String(attrSynchronizerCurrent, currentName),
		attribute.String(attrSynchronizerReason, string(changeContext.Reason())),
	))
	for _, s := range seen {
		var value int64
		if s == current {
			value = 1
		}
		h.synchronizerActive.Record(ctx, value, withSource(s))
	}
	return nil
}

// InitializationCompleted implements the InitializationCompleted handler.
func (h *MetricsHook) InitializationCompleted(
	ctx context.Context,
	initializationContext ldhooks.InitializationContext,
) error {
	outcome := initializationFailed
	if initializationContext.Succeeded() {
		outcome = initializationSucceeded
	}
	h.initializationDuration.Record(ctx, initializationContext.Duration().Seconds(),
		withSource(initializationContext.DataSource(), attribute.String(attrInitializationOutcome, outcome)))
	return nil
}

// EventFlushCompleted implements the EventFlushCompleted handler.
func (h *MetricsHook) EventFlushCompleted(ctx context.Context, flushContext ldhooks.EventFlushContext) error {
	count := int64(flushContext.EventCount())
	seconds := flushContext.Duration().Seconds()

	if dropped := flushContext.DroppedCount(); dropped > 0 {
		h.eventsDropped.Add(ctx, int64(dropped))
	}

	if flushContext.Success() {
		h.eventsSent.Add(ctx, count)
		h.eventsSentSize.Add(ctx, int64(flushContext.PayloadBytes()))
		h.eventsFlushes.Add(ctx, 1, h.successAttrs)
		h.eventsBatchSize.Record(ctx, count, h.successAttrs)
		h.eventsFlushDuration.Record(ctx, seconds, h.successAttrs)
		return nil
	}

	errorType := errorTypeOther
	errorAttrs := make([]attribute.KeyValue, 0, 3)
	if code := flushContext.StatusCode(); code > 0 {
		errorType = strconv.Itoa(code)
		errorAttrs = append(errorAttrs, attribute.Int(attrHTTPStatusCode, code))
	}
	errorAttrs = append(errorAttrs, attribute.String(attrErrorType, errorType))
	h.eventsFailed.Add(ctx, count, metric.WithAttributes(errorAttrs...))
	h.eventsFlushes.Add(ctx, 1, metric.WithAttributes(
		append(errorAttrs, attribute.String(attrFlushOutcome, flushOutcomeFailure))...))
	h.eventsBatchSize.Record(ctx, count, h.failureAttrs)
	h.eventsFlushDuration.Record(ctx, seconds, h.failureAttrs)
	return nil
}

// Ensure that MetricsHook conforms to the hook interfaces it is meant to implement.
var (
	_ ldhooks.Hook                    = (*MetricsHook)(nil)
	_ ldhooks.DataSourceStatusHandler = (*MetricsHook)(nil)
	_ ldhooks.InitializerHandler      = (*MetricsHook)(nil)
	_ ldhooks.SynchronizerHandler     = (*MetricsHook)(nil)
	_ ldhooks.InitializationHandler   = (*MetricsHook)(nil)
	_ ldhooks.EventFlushHandler       = (*MetricsHook)(nil)
)
