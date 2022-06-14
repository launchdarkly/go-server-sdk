package datasource

import "github.com/launchdarkly/go-server-sdk/v6/subsystems"

// NewNullDataSource returns a stub implementation of DataSource.
func NewNullDataSource() subsystems.DataSource {
	return nullDataSource{}
}

type nullDataSource struct{}

func (n nullDataSource) IsInitialized() bool {
	return true
}

func (n nullDataSource) Close() error {
	return nil
}

func (n nullDataSource) Start(closeWhenReady chan<- struct{}) {
	close(closeWhenReady)
}
