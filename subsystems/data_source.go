package subsystems

import (
	"context"
	"io"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
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

type InitialPayload struct {
	Data    []ldstoretypes.Collection
	Version *int
	Fresh   bool
}

type DataInitializer interface {
	Name() string
	Fetch(ctx context.Context) (*InitialPayload, error)
}

type DataSynchronizer interface {
	Sync(closeWhenReady chan struct{}, payloadVersion *int)
	io.Closer
}
