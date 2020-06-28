package sharedtest

import "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

// SingleDataStoreFactory is a test implementation of DataStoreFactory that always returns the same
// pre-existing instance.
type SingleDataStoreFactory struct {
	Instance interfaces.DataStore
}

func (f SingleDataStoreFactory) CreateDataStore( //nolint:golint
	context interfaces.ClientContext,
	dataStoreUpdates interfaces.DataStoreUpdates,
) (interfaces.DataStore, error) {
	return f.Instance, nil
}

// DataStoreFactoryThatExposesUpdater is a test implementation of DataStoreFactory that captures the
// DataStoreUpdates instance provided by LDClient.
type DataStoreFactoryThatExposesUpdater struct {
	UnderlyingFactory interfaces.DataStoreFactory
	DataStoreUpdates  interfaces.DataStoreUpdates
}

func (f *DataStoreFactoryThatExposesUpdater) CreateDataStore( //nolint:golint
	context interfaces.ClientContext,
	dataStoreUpdates interfaces.DataStoreUpdates,
) (interfaces.DataStore, error) {
	f.DataStoreUpdates = dataStoreUpdates
	return f.UnderlyingFactory.CreateDataStore(context, dataStoreUpdates)
}

// SinglePersistentDataStoreFactory is a test implementation of PersistentDataStoreFactory that always
// returns the same pre-existing instance.
type SinglePersistentDataStoreFactory struct {
	Instance interfaces.PersistentDataStore
}

func (f SinglePersistentDataStoreFactory) CreatePersistentDataStore( //nolint:golint
	context interfaces.ClientContext,
) (interfaces.PersistentDataStore, error) {
	return f.Instance, nil
}
