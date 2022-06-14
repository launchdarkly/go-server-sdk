package sharedtest

import "github.com/launchdarkly/go-server-sdk/v6/subsystems"

// SingleDataStoreFactory is a test implementation of DataStoreFactory that always returns the same
// pre-existing instance.
type SingleDataStoreFactory struct {
	Instance subsystems.DataStore
}

func (f SingleDataStoreFactory) CreateDataStore( //nolint:revive
	context subsystems.ClientContext,
	dataStoreUpdates subsystems.DataStoreUpdates,
) (subsystems.DataStore, error) {
	return f.Instance, nil
}

// DataStoreFactoryThatExposesUpdater is a test implementation of DataStoreFactory that captures the
// DataStoreUpdates instance provided by LDClient.
type DataStoreFactoryThatExposesUpdater struct {
	UnderlyingFactory subsystems.DataStoreFactory
	DataStoreUpdates  subsystems.DataStoreUpdates
}

func (f *DataStoreFactoryThatExposesUpdater) CreateDataStore( //nolint:revive
	context subsystems.ClientContext,
	dataStoreUpdates subsystems.DataStoreUpdates,
) (subsystems.DataStore, error) {
	f.DataStoreUpdates = dataStoreUpdates
	return f.UnderlyingFactory.CreateDataStore(context, dataStoreUpdates)
}

// SinglePersistentDataStoreFactory is a test implementation of PersistentDataStoreFactory that always
// returns the same pre-existing instance.
type SinglePersistentDataStoreFactory struct {
	Instance subsystems.PersistentDataStore
}

func (f SinglePersistentDataStoreFactory) CreatePersistentDataStore( //nolint:revive
	context subsystems.ClientContext,
) (subsystems.PersistentDataStore, error) {
	return f.Instance, nil
}
