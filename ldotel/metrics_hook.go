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
	flushOutcomeSucceeded = "succeeded"
	flushOutcomeFailed    = "failed"
	// errorTypeOther is the semantic-convention value for an error that has no more specific type.
	errorTypeOther = "_OTHER"
	// noneValue is the previous state of the first status. It is also the name of a synchronizer
	// that does not exist on one side of a change.
	noneValue = "none"
	// notProvidedValue is reported for a descriptor field the component did not provide.
	notProvidedValue = "not_provided"
	// dataSystemName identifies the data system itself. Statuses reported before any component is
	// known belong to it.
	dataSystemName          = "data_system"
	initializationSucceeded = "succeeded"
	initializationFailed    = "failed"
)

// dataSystemDescriptor is the reporting component before any initializer or synchronizer is known.
var dataSystemDescriptor = interfaces.DataSourceDescriptor{Name: dataSystemName} //nolint:gochecknoglobals

// allDataSourceStates lists every state the SDK can report. The observed gauges report a value for
// each of them: 1, or the time in state, for the current state and 0 for the others.
var allDataSourceStates = []interfaces.DataSourceState{ //nolint:gochecknoglobals
	interfaces.DataSourceStateInitializing,
	interfaces.DataSourceStateValid,
	interfaces.DataSourceStateInterrupted,
	interfaces.DataSourceStateOff,
}

// MetricsHookOption is used to implement functional options for the MetricsHook.
type MetricsHookOption func(hook *MetricsHook)

// WithMeterProvider sets the MeterProvider used to create instruments. By default the hook uses
// the global provider returned by otel.GetMeterProvider.
func WithMeterProvider(provider metric.MeterProvider) MetricsHookOption {
	return func(h *MetricsHook) {
		h.meterProvider = provider
	}
}

// WithAttributes adds constant attributes to every measurement the hook records. Two clients in one
// process that share a meter provider write to the same series; an attribute such as a client name
// tells them apart.
func WithAttributes(attrs ...attribute.KeyValue) MetricsHookOption {
	return func(h *MetricsHook) {
		h.constAttrs = append(h.constAttrs, attrs...)
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
// component from the initializer and synchronizer handlers, not from the status itself. The state
// gauges are observed at each collection from the hook's view of the data source.
//
// One hook instance serves one client. The client calls Close when it closes, which stops the
// observations. The hook is safe for concurrent use.
type MetricsHook struct {
	ldhooks.Unimplemented
	metadata      ldhooks.Metadata
	meterProvider metric.MeterProvider
	constAttrs    []attribute.KeyValue

	eventsSent              metric.Int64Counter
	eventsFailed            metric.Int64Counter
	eventsDropped           metric.Int64Counter
	eventsFlushes           metric.Int64Counter
	eventsBatchSize         metric.Int64Histogram
	eventsFlushDuration     metric.Float64Histogram
	eventsSentSize          metric.Int64Counter
	dataSourceErrors        metric.Int64Counter
	dataSourceTransitions   metric.Int64Counter
	dataSourceInterrupted   metric.Float64Counter
	dataSourceInterruption  metric.Float64Histogram
	initializerAttempts     metric.Int64Counter
	initializerDuration     metric.Float64Histogram
	initializationDuration  metric.Float64Histogram
	synchronizerTransitions metric.Int64Counter

	// Observed at each collection from the view below.
	dataSourceState         metric.Int64ObservableGauge
	dataSourceStateDuration metric.Float64ObservableGauge
	synchronizerActive      metric.Int64ObservableGauge

	// Pre-built attribute options for the hot paths.
	successAttrs metric.MeasurementOption
	failureAttrs metric.MeasurementOption

	mu           sync.Mutex
	registration metric.Registration
	view         dataSourceView
}

// dataSourceView is what the hook knows about the data source. The handlers update it. The
// observation callback reads a copy of it.
type dataSourceView struct {
	// component reports the statuses now. Before any initializer or synchronizer is known, it is
	// the data system itself.
	component interfaces.DataSourceDescriptor
	// componentSince is the time at which component became the reporting component.
	componentSince time.Time
	// components lists every component that has been the reporting component. The observed gauges
	// report a value for each of them, so a component that stopped reads 0.
	components []interfaces.DataSourceDescriptor
	status     interfaces.DataSourceStatus
	haveStatus bool
	// interruptedComponent is the reporting component at the time the INTERRUPTED state was entered.
	interruptedComponent interfaces.DataSourceDescriptor
	// activeSynchronizer runs now, when haveSynchronizer is true.
	activeSynchronizer interfaces.DataSourceDescriptor
	haveSynchronizer   bool
	// synchronizers lists every synchronizer that has started.
	synchronizers []interfaces.DataSourceDescriptor
}

// setComponent makes d the reporting component. The statuses that follow belong to it.
func (v *dataSourceView) setComponent(d interfaces.DataSourceDescriptor, now time.Time) {
	if d != v.component {
		v.component = d
		v.componentSince = now
	}
	v.components = appendDescriptor(v.components, d)
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

func (b *instrumentBuilder) int64ObservableGauge(name, desc, unit string) metric.Int64ObservableGauge {
	g, err := b.meter.Int64ObservableGauge(name, metric.WithDescription(desc), metric.WithUnit(unit))
	b.keep(err)
	return g
}

func (b *instrumentBuilder) float64ObservableGauge(name, desc, unit string) metric.Float64ObservableGauge {
	g, err := b.meter.Float64ObservableGauge(name, metric.WithDescription(desc), metric.WithUnit(unit))
	b.keep(err)
	return g
}

func (b *instrumentBuilder) int64Histogram(name, desc, unit string, buckets ...float64) metric.Int64Histogram {
	hg, err := b.meter.Int64Histogram(name, metric.WithDescription(desc), metric.WithUnit(unit),
		metric.WithExplicitBucketBoundaries(buckets...))
	b.keep(err)
	return hg
}

// secondsHistogram creates a histogram of durations in seconds.
func (b *instrumentBuilder) secondsHistogram(name, desc string, buckets ...float64) metric.Float64Histogram {
	hg, err := b.meter.Float64Histogram(name, metric.WithDescription(desc), metric.WithUnit("s"),
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
		view:     dataSourceView{component: dataSystemDescriptor},
	}
	for _, opt := range opts {
		opt(h)
	}
	if h.meterProvider == nil {
		// The global provider is a delegate. It follows the provider that the application sets
		// later, so the instruments can be created now.
		h.meterProvider = otel.GetMeterProvider()
	}
	b := &instrumentBuilder{meter: h.meterProvider.Meter(meterName)}

	h.eventsSent = b.int64Counter(metricEventsSent,
		"Analytics events accepted by the LaunchDarkly events service", "{event}")
	h.eventsFailed = b.int64Counter(metricEventsFailed,
		"Analytics events lost because a batch could not be delivered after all retries", "{event}")
	h.eventsDropped = b.int64Counter(metricEventsDropped,
		"Analytics events discarded before delivery because the SDK event buffer was full, reported with the next flush",
		"{event}")
	h.eventsFlushes = b.int64Counter(metricEventsFlushes,
		"Attempts to deliver a batch of analytics events", "{flush}")
	h.eventsBatchSize = b.int64Histogram(metricEventsBatchSize,
		"Number of analytics events in each delivered or failed batch", "{event}",
		1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 20000)
	h.eventsFlushDuration = b.secondsHistogram(metricEventsFlushDuration,
		"Time taken to deliver a batch of analytics events, including any retry",
		0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60)
	h.eventsSentSize = b.int64Counter(metricEventsSentSize,
		"Bytes of analytics event payloads accepted by the LaunchDarkly events service, before compression", "By")
	h.dataSourceErrors = b.int64Counter(metricDataSourceErrors,
		"Errors reported by the data source while connecting to or reading from LaunchDarkly", "{error}")
	h.dataSourceTransitions = b.int64Counter(metricDataSourceTransitions,
		"Data source state changes", "{transition}")
	h.dataSourceInterrupted = b.float64Counter(metricDataSourceInterrupted,
		"Total time spent in completed INTERRUPTED periods", "s")
	h.dataSourceInterruption = b.secondsHistogram(metricDataSourceInterruption,
		"Duration of each completed INTERRUPTED period",
		1, 5, 10, 30, 60, 120, 300, 600, 1800, 3600)
	h.initializerAttempts = b.int64Counter(metricInitializerAttempts,
		"Attempts to obtain initial flag data from a data initializer, by outcome", "{attempt}")
	h.initializerDuration = b.secondsHistogram(metricInitializerDuration,
		"Time taken by each data initializer attempt",
		0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60)
	h.initializationDuration = b.secondsHistogram(metricInitializationDuration,
		"Time from data system start until the SDK first had data or gave up",
		0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120)
	h.synchronizerTransitions = b.int64Counter(metricSynchronizerTransitions,
		"Changes of the active data synchronizer, by reason", "{transition}")
	h.dataSourceState = b.int64ObservableGauge(metricDataSourceState,
		"1 for the current state of the reporting component, 0 for every other state", "{state}")
	h.dataSourceStateDuration = b.float64ObservableGauge(metricDataSourceStateAge,
		"Time the reporting component has spent in its current state", "s")
	h.synchronizerActive = b.int64ObservableGauge(metricSynchronizerActive,
		"1 for the running data synchronizer, 0 for every other synchronizer that has run", "{synchronizer}")
	if b.err != nil {
		return nil, b.err
	}
	registration, err := b.meter.RegisterCallback(h.observe,
		h.dataSourceState, h.dataSourceStateDuration, h.synchronizerActive)
	if err != nil {
		return nil, err
	}
	h.registration = registration

	h.successAttrs = h.attrs(attribute.String(attrFlushOutcome, flushOutcomeSucceeded))
	h.failureAttrs = h.attrs(attribute.String(attrFlushOutcome, flushOutcomeFailed))
	return h, nil
}

// Close stops the observation of the state gauges. The client calls Close when it closes, so that a
// closed client does not report its last state at each later collection. Close can be called more
// than once.
func (h *MetricsHook) Close() error {
	h.mu.Lock()
	registration := h.registration
	h.registration = nil
	h.mu.Unlock()
	if registration == nil {
		return nil
	}
	return registration.Unregister()
}

// attrs joins the constant attributes with the attributes of one measurement.
func (h *MetricsHook) attrs(kvs ...attribute.KeyValue) metric.MeasurementOption {
	if len(h.constAttrs) == 0 {
		return metric.WithAttributes(kvs...)
	}
	all := make([]attribute.KeyValue, 0, len(h.constAttrs)+len(kvs))
	all = append(all, h.constAttrs...)
	all = append(all, kvs...)
	return metric.WithAttributes(all...)
}

// withSource returns the attributes of a measurement about one data source component.
func (h *MetricsHook) withSource(
	d interfaces.DataSourceDescriptor,
	extra ...attribute.KeyValue,
) metric.MeasurementOption {
	return h.attrs(append(sourceAttributes(d), extra...)...)
}

// sourceAttributes returns the attributes that identify a data source component. Fields the
// component did not provide are reported as "not_provided" so that series carry a consistent key set.
func sourceAttributes(d interfaces.DataSourceDescriptor) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String(attrDataSourceName, orNotProvided(d.Name)),
		attribute.String(attrDataSourceProtocol, orNotProvided(string(d.Protocol))),
		attribute.String(attrDataSourceTransport, orNotProvided(string(d.Transport))),
	}
}

func orNotProvided(value string) string {
	if value == "" {
		return notProvidedValue
	}
	return value
}

// synchronizerName returns the name attribute value for one side of a synchronizer change.
func synchronizerName(d interfaces.DataSourceDescriptor) string {
	if !d.IsDefined() {
		return noneValue
	}
	return orNotProvided(d.Name)
}

func appendDescriptor(
	list []interfaces.DataSourceDescriptor,
	d interfaces.DataSourceDescriptor,
) []interfaces.DataSourceDescriptor {
	for _, item := range list {
		if item == d {
			return list
		}
	}
	return append(list, d)
}

func laterOf(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// nonNegativeSeconds converts a computed time to seconds. A clock that steps backwards can produce
// a negative time, which a counter does not accept.
func nonNegativeSeconds(d time.Duration) float64 {
	if d < 0 {
		return 0
	}
	return d.Seconds()
}

// observe reports the state gauges from the hook's view of the data source. The reader calls it
// at each collection on its own goroutine, so it works from a copy taken under the lock. The view
// only appends to its lists, so the copy stays valid without cloning them.
func (h *MetricsHook) observe(_ context.Context, o metric.Observer) error {
	h.mu.Lock()
	view := h.view
	h.mu.Unlock()
	now := time.Now()

	if view.haveStatus {
		for _, component := range view.components {
			for _, state := range allDataSourceStates {
				var active int64
				seconds := 0.0
				if component == view.component && state == view.status.State {
					active = 1
					// A component that took over during a state counts its time from the takeover.
					since := laterOf(view.status.StateSince, view.componentSince)
					seconds = nonNegativeSeconds(now.Sub(since))
				}
				attrs := h.withSource(component, stateAttribute(state))
				o.ObserveInt64(h.dataSourceState, active, attrs)
				o.ObserveFloat64(h.dataSourceStateDuration, seconds, attrs)
			}
		}
	}
	for _, synchronizer := range view.synchronizers {
		var active int64
		if view.haveSynchronizer && synchronizer == view.activeSynchronizer {
			active = 1
		}
		o.ObserveInt64(h.synchronizerActive, active, h.withSource(synchronizer))
	}
	return nil
}

// DataSourceStatusChanged implements the DataSourceStatusChanged handler.
func (h *MetricsHook) DataSourceStatusChanged(
	ctx context.Context,
	statusContext ldhooks.DataSourceStatusContext,
) error {
	previous, current := statusContext.Previous(), statusContext.Current()
	entersInterrupted := current.State == interfaces.DataSourceStateInterrupted &&
		previous.State != interfaces.DataSourceStateInterrupted

	h.mu.Lock()
	component := h.view.component
	h.view.components = appendDescriptor(h.view.components, component)
	if entersInterrupted {
		h.view.interruptedComponent = component
	}
	interruptedComponent := h.view.interruptedComponent
	h.view.status = current
	h.view.haveStatus = true
	h.mu.Unlock()

	if current.State != previous.State {
		previousName := noneValue
		if previous.State != "" {
			previousName = string(previous.State)
		}
		h.dataSourceTransitions.Add(ctx, 1, h.withSource(component,
			stateAttribute(current.State),
			attribute.String(attrPreviousState, previousName),
		))
		if previous.State == interfaces.DataSourceStateInterrupted {
			// The interruption belongs to the component that failed, not to one that took over
			// while it lasted.
			if !interruptedComponent.IsDefined() {
				interruptedComponent = component
			}
			seconds := nonNegativeSeconds(current.StateSince.Sub(previous.StateSince))
			h.dataSourceInterrupted.Add(ctx, seconds, h.withSource(interruptedComponent))
			h.dataSourceInterruption.Record(ctx, seconds, h.withSource(interruptedComponent))
		}
	}

	if current.LastError.Kind != "" && current.LastError != previous.LastError {
		h.dataSourceErrors.Add(ctx, 1, h.withSource(component, errorAttributes(current.LastError)...))
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
		// The statuses that follow belong to the initializer whose data was applied.
		h.mu.Lock()
		h.view.setComponent(initializerContext.DataSource(), time.Now())
		h.mu.Unlock()
	}
	attrs := h.withSource(initializerContext.DataSource(),
		attribute.String(attrInitializerOutcome, string(initializerContext.Outcome())))
	h.initializerAttempts.Add(ctx, 1, attrs)
	h.initializerDuration.Record(ctx, initializerContext.Duration().Seconds(), attrs)
	return nil
}

// SynchronizerChanged implements the SynchronizerChanged handler.
func (h *MetricsHook) SynchronizerChanged(ctx context.Context, changeContext ldhooks.SynchronizerChangeContext) error {
	previous, current := changeContext.Previous(), changeContext.Current()

	h.mu.Lock()
	if current.IsDefined() {
		h.view.setComponent(current, time.Now())
		h.view.activeSynchronizer = current
		h.view.haveSynchronizer = true
		h.view.synchronizers = appendDescriptor(h.view.synchronizers, current)
	} else {
		// No synchronizer is left. The last component keeps reporting the statuses that follow.
		h.view.haveSynchronizer = false
	}
	h.mu.Unlock()

	h.synchronizerTransitions.Add(ctx, 1, h.attrs(
		attribute.String(attrSynchronizerPrevious, synchronizerName(previous)),
		attribute.String(attrSynchronizerCurrent, synchronizerName(current)),
		attribute.String(attrSynchronizerReason, string(changeContext.Reason())),
	))
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
		h.withSource(initializationContext.DataSource(), attribute.String(attrInitializationOutcome, outcome)))
	return nil
}

// EventFlushCompleted implements the EventFlushCompleted handler.
func (h *MetricsHook) EventFlushCompleted(ctx context.Context, flushContext ldhooks.EventFlushContext) error {
	count := int64(flushContext.EventCount())
	seconds := flushContext.Duration().Seconds()

	if dropped := flushContext.DroppedCount(); dropped > 0 {
		h.eventsDropped.Add(ctx, int64(dropped), h.attrs())
	}

	if flushContext.Success() {
		h.eventsSent.Add(ctx, count, h.attrs())
		h.eventsSentSize.Add(ctx, int64(flushContext.PayloadBytes()), h.attrs())
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
	h.eventsFailed.Add(ctx, count, h.attrs(errorAttrs...))
	h.eventsFlushes.Add(ctx, 1, h.attrs(
		append(errorAttrs, attribute.String(attrFlushOutcome, flushOutcomeFailed))...))
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
