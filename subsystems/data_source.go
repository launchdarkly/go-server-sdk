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

type Basis struct {
	Data     []fdv2proto.Event
	Selector *fdv2proto.Selector
	Persist  bool
}

type DataInitializer interface {
	Name() string
	Fetch(ctx context.Context) (*Basis, error)
}

type DataSynchronizer interface {
	DataInitializer
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

func AsInitializer(cc ComponentConfigurer[DataSynchronizer]) ComponentConfigurer[DataInitializer] {
	return toInitializer{cc: cc}
}
