package ldevents

import (
	"net/http"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
)

const DefaultDiagnosticRecordingInterval = 15 * time.Minute
const DefaultFlushInterval = 5 * time.Second
const DefaultUserKeysFlushInterval = 5 * time.Minute

// EventsConfiguration contains options affecting the behavior of the events engine.
type EventsConfiguration struct {
	// Sets whether or not all user attributes (other than the key) should be hidden from LaunchDarkly. If this
	// is true, all user attribute values will be private, not just the attributes specified in PrivateAttributeNames.
	AllAttributesPrivate bool
	// The capacity of the events buffer. The client buffers up to this many events in memory before flushing.
	// If the capacity is exceeded before the buffer is flushed, events will be discarded.
	Capacity int
	// The interval at which periodic diagnostic events will be sent, if DiagnosticsManager is non-nil.
	DiagnosticRecordingInterval time.Duration
	DiagnosticURI               string
	DiagnosticsManager          *DiagnosticsManager
	EventsURI                   string
	// The time between flushes of the event buffer. Decreasing the flush interval means that the event buffer
	// is less likely to reach capacity.
	FlushInterval time.Duration
	Headers       http.Header
	HTTPClient    *http.Client
	// Set to true if you need to see the full user details in every analytics event.
	InlineUsersInEvents bool
	// Configures the SDK's logging behavior. You may call its SetBaseLogger() method to specify the
	// output destination (the default is standard error), and SetMinLevel() to specify the minimum level
	// of messages to be logged (the default is ldlog.Info).
	Loggers            ldlog.Loggers
	LogUserKeyInErrors bool
	// Marks a set of user attribute names private. Any users sent to LaunchDarkly with this configuration
	// active will have attributes with these names removed.
	PrivateAttributeNames []string
	SDKKey                string
	// The number of user keys that the event processor can remember at any one time, so that
	// duplicate user details will not be sent in analytics events.
	UserKeysCapacity int
	// The interval at which the event processor will reset its set of known user keys.
	UserKeysFlushInterval time.Duration
}
