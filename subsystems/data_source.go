package subsystems

import (
	"context"
	"io"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
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
	// Events is a series of events representing actions applied to data items.
	Events []fdv2proto.Event
	// Selector identifies this basis.
	Selector *fdv2proto.Selector
	// Persist is true if the data source requests that the data store persist the items to any connected
	// Persistent Stores.
	Persist bool
}

// DataInitializer represents a component capable of obtaining a Basis via a synchronous call.
type DataInitializer interface {
	// Name returns the name of the data initializer.
	Name() string
	// Fetch returns a Basis, or an error if the Basis could not be retrieved. If the context has expired,
	// return the context's error.
	Fetch(ctx context.Context) (*Basis, error)
}

// DataSynchronizer represents a component capable of obtaining a Basis and subsequent delta updates asynchronously.
type DataSynchronizer interface {
	DataInitializer
	// Sync tells the data synchronizer to begin synchronizing data, starting from an optional fdv2proto.Selector.
	// The selector may be nil indicating that a full Basis should be fetched.
	Sync(closeWhenReady chan<- struct{}, selector *fdv2proto.Selector)
	// IsInitialized returns true if the data source has successfully initialized at some point.
	//
	// Once this is true, it should remain true even if a problem occurs later.
	IsInitialized() bool
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
