package ldcomponents

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

type inMemoryDataStoreFactory struct{}

// DataStoreFactory implementation
func (f inMemoryDataStoreFactory) CreateDataStore(context interfaces.ClientContext) (interfaces.DataStore, error) {
	loggers := context.GetLoggers()
	loggers.SetPrefix("InMemoryDataStore:")
	return internal.NewInMemoryDataStore(loggers), nil
}

// DiagnosticDescription implementation
func (f inMemoryDataStoreFactory) DescribeConfiguration() ldvalue.Value {
	return ldvalue.String("memory")
}

// InMemoryDataStore returns the default in-memory DataStore implementation factory.
func InMemoryDataStore() interfaces.DataStoreFactory {
	return inMemoryDataStoreFactory{}
}
