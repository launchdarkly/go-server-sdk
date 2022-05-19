package ldcomponents

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

type inMemoryDataStoreFactory struct{}

// DataStoreFactory implementation
func (f inMemoryDataStoreFactory) CreateDataStore(
	context subsystems.ClientContext,
	dataStoreUpdates subsystems.DataStoreUpdates,
) (subsystems.DataStore, error) {
	loggers := context.GetLogging().Loggers
	loggers.SetPrefix("InMemoryDataStore:")
	return datastore.NewInMemoryDataStore(loggers), nil
}

// DiagnosticDescription implementation
func (f inMemoryDataStoreFactory) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.String("memory")
}

// InMemoryDataStore returns the default in-memory DataStore implementation factory.
func InMemoryDataStore() subsystems.DataStoreFactory {
	return inMemoryDataStoreFactory{}
}
