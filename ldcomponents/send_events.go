package ldcomponents

import (
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldattr"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v2"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal"
	"github.com/launchdarkly/go-server-sdk/v6/internal/endpoints"
)

const (
	// DefaultEventsBaseURI is the default value for EventProcessorBuilder.BaseURI.
	DefaultEventsBaseURI = "https://events.launchdarkly.com"
	// DefaultEventsCapacity is the default value for EventProcessorBuilder.Capacity.
	DefaultEventsCapacity = 10000
	// DefaultDiagnosticRecordingInterval is the default value for EventProcessorBuilder.DiagnosticRecordingInterval.
	DefaultDiagnosticRecordingInterval = 15 * time.Minute
	// DefaultFlushInterval is the default value for EventProcessorBuilder.FlushInterval.
	DefaultFlushInterval = 5 * time.Second
	// DefaultContextKeysCapacity is the default value for EventProcessorBuilder.ContextKeysCapacity.
	DefaultContextKeysCapacity = 1000
	// DefaultContextKeysFlushInterval is the default value for EventProcessorBuilder.ContextKeysFlushInterval.
	DefaultContextKeysFlushInterval = 5 * time.Minute
	// MinimumDiagnosticRecordingInterval is the minimum value for EventProcessorBuilder.DiagnosticRecordingInterval.
	MinimumDiagnosticRecordingInterval = 60 * time.Second
)

// EventProcessorBuilder provides methods for configuring analytics event behavior.
//
// See SendEvents for usage.
type EventProcessorBuilder struct {
	allAttributesPrivate        bool
	baseURI                     string
	capacity                    int
	diagnosticRecordingInterval time.Duration
	flushInterval               time.Duration
	logContextKeyInErrors       bool
	privateAttributes           []ldattr.Ref
	contextKeysCapacity         int
	contextKeysFlushInterval    time.Duration
}

// SendEvents returns a configuration builder for analytics event delivery.
//
// The default configuration has events enabled with default settings. If you want to customize this
// behavior, call this method to obtain a builder, change its properties with the EventProcessorBuilder
// methods, and store it in Config.Events:
//
//     config := ld.Config{
//         Events: ldcomponents.SendEvents().Capacity(5000).FlushInterval(2 * time.Second),
//     }
//
// To disable analytics events, use NoEvents instead of SendEvents.
func SendEvents() *EventProcessorBuilder {
	return &EventProcessorBuilder{
		capacity:                    DefaultEventsCapacity,
		diagnosticRecordingInterval: DefaultDiagnosticRecordingInterval,
		flushInterval:               DefaultFlushInterval,
		contextKeysCapacity:         DefaultContextKeysCapacity,
		contextKeysFlushInterval:    DefaultContextKeysFlushInterval,
	}
}

// CreateEventProcessor is called by the SDK to create the event processor instance.
func (b *EventProcessorBuilder) CreateEventProcessor(
	context interfaces.ClientContext,
) (ldevents.EventProcessor, error) {
	loggers := context.GetLogging().GetLoggers()

	configuredBaseURI := endpoints.SelectBaseURI(
		context.GetBasic().ServiceEndpoints,
		endpoints.EventsService,
		b.baseURI,
		loggers,
	)

	eventSender := ldevents.NewServerSideEventSender(context.GetHTTP().CreateHTTPClient(),
		context.GetBasic().SDKKey, configuredBaseURI, context.GetHTTP().GetDefaultHeaders(), loggers)
	eventsConfig := ldevents.EventsConfiguration{
		AllAttributesPrivate:        b.allAttributesPrivate,
		Capacity:                    b.capacity,
		DiagnosticRecordingInterval: b.diagnosticRecordingInterval,
		EventSender:                 eventSender,
		FlushInterval:               b.flushInterval,
		Loggers:                     loggers,
		LogUserKeyInErrors:          b.logContextKeyInErrors,
		PrivateAttributes:           b.privateAttributes,
		UserKeysCapacity:            b.contextKeysCapacity,
		UserKeysFlushInterval:       b.contextKeysFlushInterval,
	}
	if cci, ok := context.(*internal.ClientContextImpl); ok {
		eventsConfig.DiagnosticsManager = cci.DiagnosticsManager
	}
	return ldevents.NewDefaultEventProcessor(eventsConfig), nil
}

// AllAttributesPrivate sets whether or not all optional context attributes should be hidden from LaunchDarkly.
//
// If this is true, all context attribute values (other than the key) will be private, not just the attributes
// specified with PrivateAttributeNames or on a per-context basis with ldcontext.Builder methods. By default,
// it is false.
func (b *EventProcessorBuilder) AllAttributesPrivate(value bool) *EventProcessorBuilder {
	b.allAttributesPrivate = value
	return b
}

// Capacity sets the capacity of the events buffer.
//
// The client buffers up to this many events in memory before flushing. If the capacity is exceeded before
// the buffer is flushed (see FlushInterval), events will be discarded. Increasing the capacity means that
// events are less likely to be discarded, at the cost of consuming more memory.
//
// The default value is DefaultEventsCapacity.
func (b *EventProcessorBuilder) Capacity(capacity int) *EventProcessorBuilder {
	b.capacity = capacity
	return b
}

// DiagnosticRecordingInterval sets the interval at which periodic diagnostic data is sent.
//
// The default value is DefaultDiagnosticRecordingInterval; the minimum value is MinimumDiagnosticRecordingInterval.
// This property is ignored if Config.DiagnosticOptOut is set to true.
func (b *EventProcessorBuilder) DiagnosticRecordingInterval(interval time.Duration) *EventProcessorBuilder {
	if interval < MinimumDiagnosticRecordingInterval {
		b.diagnosticRecordingInterval = MinimumDiagnosticRecordingInterval
	} else {
		b.diagnosticRecordingInterval = interval
	}
	return b
}

// FlushInterval sets the interval between flushes of the event buffer.
//
// Decreasing the flush interval means that the event buffer is less likely to reach capacity (see Capacity).
//
// The default value is DefaultFlushInterval.
func (b *EventProcessorBuilder) FlushInterval(interval time.Duration) *EventProcessorBuilder {
	b.flushInterval = interval
	return b
}

// PrivateAttributes marks a set of attribute names as always private.
//
// Any contexts sent to LaunchDarkly with this configuration active will have attributes with these
// names removed. This is in addition to any attributes that were marked as private for an individual
// context with ldcontext.Builder methods. Setting AllAttributesPrivate to true overrides this.
//
//     config := ld.Config{
//         Events: ldcomponents.SendEvents().
//             PrivateAttributeNames("email", "some-custom-attribute"),
//     }
//
// If and only if a parameter starts with a slash, it is interpreted as a slash-delimited path that
// can denote a nested property within a JSON object. For instance, "/address/street" means that if
// there is an attribute called "address" that is a JSON object, and one of the object's properties
// is "street", the "street" property will be redacted from the analytics data but other properties
// within "address" will still be sent. This syntax also uses the JSON Pointer convention of escaping
// a literal slash character as "~1" and a tilde as "~0".
//
// This method replaces any previous parameters that were set on the same builder with
// PrivateAttributes, rather than adding to them.
func (b *EventProcessorBuilder) PrivateAttributes(attributes ...string) *EventProcessorBuilder {
	b.privateAttributes = make([]ldattr.Ref, 0, len(attributes))
	for _, a := range attributes {
		b.privateAttributes = append(b.privateAttributes, ldattr.NewRef(a))
	}
	return b
}

// ContextKeysCapacity sets the number of context keys that the event processor can remember at any one
// time.
//
// To avoid sending duplicate context details in analytics events, the SDK maintains a cache of recently
// seen context keys, expiring at an interval set by ContextKeysFlushInterval.
//
// The default value is DefaultContextKeysCapacity.
func (b *EventProcessorBuilder) ContextKeysCapacity(contextKeysCapacity int) *EventProcessorBuilder {
	b.contextKeysCapacity = contextKeysCapacity
	return b
}

// ContextKeysFlushInterval sets the interval at which the event processor will reset its cache of known context keys.
//
// The default value is DefaultContextKeysFlushInterval.
func (b *EventProcessorBuilder) ContextKeysFlushInterval(interval time.Duration) *EventProcessorBuilder {
	b.contextKeysFlushInterval = interval
	return b
}

// DescribeConfiguration is used internally by the SDK to inspect the configuration.
func (b *EventProcessorBuilder) DescribeConfiguration(context interfaces.ClientContext) ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("allAttributesPrivate", ldvalue.Bool(b.allAttributesPrivate)).
		Set("customEventsURI", ldvalue.Bool(
			endpoints.IsCustom(context.GetBasic().ServiceEndpoints, endpoints.EventsService, b.baseURI))).
		Set("diagnosticRecordingIntervalMillis", durationToMillisValue(b.diagnosticRecordingInterval)).
		Set("eventsCapacity", ldvalue.Int(b.capacity)).
		Set("eventsFlushIntervalMillis", durationToMillisValue(b.flushInterval)).
		Set("userKeysCapacity", ldvalue.Int(b.contextKeysCapacity)).
		Set("userKeysFlushIntervalMillis", durationToMillisValue(b.contextKeysFlushInterval)).
		Build()
}

func durationToMillisValue(d time.Duration) ldvalue.Value {
	return ldvalue.Float64(float64(uint64(d / time.Millisecond)))
}
