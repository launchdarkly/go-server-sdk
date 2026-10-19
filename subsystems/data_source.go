package subsystems

import (
	"context"
	"io"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// DataSource describes the interface for an object that receives feature flag data.
type DataSource interface {
	io.Closer

	// IsInitialized returns true if the data source has successfully initialized at some point.
	//
	// Once this is true, it should remain true even if a problem occurs later.
	IsInitialized() bool

	// Start tells the data source to begin initializing. It should not try to make any connections
	// or do any other significant activity until Start is called.
	//
	// The data source should close the closeWhenReady channel if and when it has either successfully
	// initialized for the first time, or determined that initialization cannot ever succeed.
	Start(closeWhenReady chan<- struct{})
}

// Basis represents the initial payload of data that a data source can provide. Initializers provide this
// via Fetch, whereas Synchronizers provide it asynchronously via the injected DataDestination.
type Basis struct {
	ChangeSet ChangeSet
	// Persist is true if the data source requests that the data store persist the items to any connected
	// Persistent Stores.
	Persist       bool
	EnvironmentID ldvalue.OptionalString
}

// DataSelector is an interface that provides a Selector, which is used by synchronizers and initializers to
// the basis of data to fetch.
type DataSelector interface {
	Selector() Selector
}

// DataInitializer represents a component capable of obtaining a Basis via a synchronous call.
type DataInitializer interface {
	// Name returns the name of the data initializer.
	Name() string
	// Fetch returns a Basis, or an error if the Basis could not be retrieved. If the context has
	// expired, it returns the context's error. If the LaunchDarkly server has instructed the SDK
	// to fall back to the FDv1 protocol, fdv1Fallback.Requested is true; in that case the Basis
	// may still carry data from the same response, and err may be nil or carry the underlying
	// HTTP error for logging. Callers should branch on fdv1Fallback before err.
	Fetch(ds DataSelector, ctx context.Context) (basis *Basis, fdv1Fallback FDv1Fallback, err error)
}

// FDv1Fallback is an instruction from LaunchDarkly to stop using the FDv2 protocol and fall back
// to FDv1 for a while. The zero value means that no fallback was requested.
type FDv1Fallback struct {
	// Requested is true if LaunchDarkly asked the SDK to fall back to FDv1.
	Requested bool
	// TTL is how long LaunchDarkly asked the SDK to stay on FDv1 before trying FDv2 again. Zero
	// means that LaunchDarkly did not supply a usable TTL, so the SDK uses its default.
	TTL time.Duration
}

// DataSynchronizerResult represents the results of a Synchronizer's ongoing Sync method.
type DataSynchronizerResult struct {
	ChangeSet      *ChangeSet
	State          interfaces.DataSourceState
	Error          interfaces.DataSourceErrorInfo
	FallbackToFDv1 bool
	// FDv1FallbackTTL is how long to stay on FDv1 before trying FDv2 again, when FallbackToFDv1
	// is true. Zero means that LaunchDarkly did not supply a usable TTL, so the SDK uses its
	// default.
	FDv1FallbackTTL time.Duration
	EnvironmentID   ldvalue.OptionalString
}

// DataSynchronizer represents a component capable of obtaining a Basis and subsequent delta updates asynchronously.
type DataSynchronizer interface {
	DataInitializer
	// Sync tells the data synchronizer to begin synchronizing data.
	Sync(DataSelector) <-chan DataSynchronizerResult
	io.Closer
}

type toInitializer struct {
	cc ComponentConfigurer[DataSynchronizer]
}

func (t toInitializer) Build(context ClientContext) (DataInitializer, error) {
	sync, err := t.cc.Build(context)
	if err != nil {
		return nil, err
	}
	return sync, nil
}

// AsInitializer is a helper method that converts a DataSynchronizer to a DataInitializer. This is useful because
// DataSynchronizers are generally also DataInitializers, so it's possible to use one for that purpose if the
// situation calls for it. The primary example is using a Polling synchronizer as an initializer.
func AsInitializer(cc ComponentConfigurer[DataSynchronizer]) ComponentConfigurer[DataInitializer] {
	return toInitializer{cc: cc}
}
