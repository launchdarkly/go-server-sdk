package ldhooks

import (
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// InitializerOutcome describes how an attempt to initialize from one data initializer ended.
type InitializerOutcome string

const (
	// InitializerOutcomeSucceeded means the initializer provided data with a selector. Initialization
	// is complete.
	InitializerOutcomeSucceeded InitializerOutcome = "succeeded"

	// InitializerOutcomeSucceededWithoutSelector means the initializer provided data that the SDK
	// applied, but the data carries no selector. The SDK continues with the next initializer or a
	// synchronizer.
	InitializerOutcomeSucceededWithoutSelector InitializerOutcome = "succeeded_without_selector"

	// InitializerOutcomeNoData means the initializer returned a payload with no usable data.
	InitializerOutcomeNoData InitializerOutcome = "no_data"

	// InitializerOutcomeFailed means the initializer returned an error.
	InitializerOutcomeFailed InitializerOutcome = "failed"

	// InitializerOutcomeFallback means the initializer asked the SDK to fall back to the FDv1 protocol.
	InitializerOutcomeFallback InitializerOutcome = "fallback"

	// InitializerOutcomeCancelled means the SDK was closed while the initializer was running.
	InitializerOutcomeCancelled InitializerOutcome = "cancelled"
)

// InitializerContext contains the information passed to the InitializerCompleted handler.
type InitializerContext struct {
	dataSource interfaces.DataSourceDescriptor
	outcome    InitializerOutcome
	err        error
	duration   time.Duration
	applied    bool
}

// NewInitializerContext creates an InitializerContext. This is called by the SDK.
func NewInitializerContext(
	dataSource interfaces.DataSourceDescriptor,
	outcome InitializerOutcome,
	err error,
	duration time.Duration,
	applied bool,
) InitializerContext {
	return InitializerContext{dataSource: dataSource, outcome: outcome, err: err, duration: duration, applied: applied}
}

// DataSource identifies the initializer.
func (c InitializerContext) DataSource() interfaces.DataSourceDescriptor { return c.dataSource }

// Outcome returns how the attempt ended.
func (c InitializerContext) Outcome() InitializerOutcome { return c.outcome }

// Error returns the error the initializer reported, or nil.
func (c InitializerContext) Error() error { return c.err }

// Duration returns how long the attempt took.
func (c InitializerContext) Duration() time.Duration { return c.duration }

// Applied returns true if the SDK applied data from this initializer to its store.
func (c InitializerContext) Applied() bool { return c.applied }

// SynchronizerChangeReason describes why the active synchronizer changed.
type SynchronizerChangeReason string

const (
	// SynchronizerChangeReasonInitial means the first synchronizer started.
	SynchronizerChangeReasonInitial SynchronizerChangeReason = "initial"

	// SynchronizerChangeReasonFallback means the previous synchronizer was unhealthy for too long and
	// the SDK moved to the next one.
	SynchronizerChangeReasonFallback SynchronizerChangeReason = "fallback"

	// SynchronizerChangeReasonRecover means a fallback synchronizer was healthy for long enough and
	// the SDK returned to the primary one.
	SynchronizerChangeReasonRecover SynchronizerChangeReason = "recover"

	// SynchronizerChangeReasonRemoved means the previous synchronizer failed permanently and will not
	// be tried again.
	SynchronizerChangeReasonRemoved SynchronizerChangeReason = "removed"

	// SynchronizerChangeReasonFDv1Fallback means LaunchDarkly asked the SDK to use the FDv1 protocol.
	SynchronizerChangeReasonFDv1Fallback SynchronizerChangeReason = "fdv1_fallback"

	// SynchronizerChangeReasonExhausted means no synchronizers remain. Current is empty.
	SynchronizerChangeReasonExhausted SynchronizerChangeReason = "exhausted"
)

// SynchronizerChangeContext contains the information passed to the SynchronizerChanged handler.
type SynchronizerChangeContext struct {
	previous  interfaces.DataSourceDescriptor
	current   interfaces.DataSourceDescriptor
	reason    SynchronizerChangeReason
	lastError interfaces.DataSourceErrorInfo
}

// NewSynchronizerChangeContext creates a SynchronizerChangeContext. This is called by the SDK.
func NewSynchronizerChangeContext(
	previous, current interfaces.DataSourceDescriptor,
	reason SynchronizerChangeReason,
	lastError interfaces.DataSourceErrorInfo,
) SynchronizerChangeContext {
	return SynchronizerChangeContext{previous: previous, current: current, reason: reason, lastError: lastError}
}

// Previous identifies the synchronizer that stopped. It is empty for the first synchronizer.
func (c SynchronizerChangeContext) Previous() interfaces.DataSourceDescriptor { return c.previous }

// Current identifies the synchronizer that started. It is empty when no synchronizers remain.
func (c SynchronizerChangeContext) Current() interfaces.DataSourceDescriptor { return c.current }

// Reason returns why the change happened.
func (c SynchronizerChangeContext) Reason() SynchronizerChangeReason { return c.reason }

// LastError returns the most recent data source error at the time of the change, if any.
func (c SynchronizerChangeContext) LastError() interfaces.DataSourceErrorInfo { return c.lastError }

// InitializationContext contains the information passed to the InitializationCompleted handler.
type InitializationContext struct {
	dataSource interfaces.DataSourceDescriptor
	duration   time.Duration
	succeeded  bool
}

// NewInitializationContext creates an InitializationContext. This is called by the SDK.
func NewInitializationContext(
	dataSource interfaces.DataSourceDescriptor,
	duration time.Duration,
	succeeded bool,
) InitializationContext {
	return InitializationContext{dataSource: dataSource, duration: duration, succeeded: succeeded}
}

// DataSource identifies the component that made data available, or the last component that was
// tried when initialization did not succeed.
func (c InitializationContext) DataSource() interfaces.DataSourceDescriptor { return c.dataSource }

// Duration returns the time from the start of the data system to the completion of initialization.
func (c InitializationContext) Duration() time.Duration { return c.duration }

// Succeeded returns true if the SDK has flag data. False means the SDK gave up and serves defaults.
func (c InitializationContext) Succeeded() bool { return c.succeeded }
