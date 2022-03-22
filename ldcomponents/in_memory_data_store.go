package ldcomponents

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
)

type inMemoryDataStoreFactory struct{}

// DataStoreFactory implementation
func (f inMemoryDataStoreFactory) CreateDataStore(
	context interfaces.ClientContext,
	dataStoreUpdates interfaces.DataStoreUpdates,
) (interfaces.DataStore, error) {
	loggers := context.GetLogging().Loggers
	loggers.SetPrefix("InMemoryDataStore:")
	return datastore.NewInMemoryDataStore(loggers), nil
}

// DiagnosticDescription implementation
func (f inMemoryDataStoreFactory) DescribeConfiguration(context interfaces.ClientContext) ldvalue.Value {
	return ldvalue.String("memory")
}

// InMemoryDataStore returns the default in-memory DataStore implementation factory.
func InMemoryDataStore() interfaces.DataStoreFactory {
	return inMemoryDataStoreFactory{}
}
