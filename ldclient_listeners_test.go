package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

// This file contains tests for all of the event broadcaster/listener functionality in the client, plus
// related methods for looking at the same kinds of status values that can be broadcast to listeners.
// It uses mock implementations of the data source and data store, so that it is only the status
// monitoring mechanisms that are being tested, not the status behavior of specific real components.
//
// Parts of this functionality are also covered by lower-level component tests like
// DataSourceUpdatesImplTest. However, the tests here verify that the client is wiring the components
// together correctly so that they work from an application's point of view.

func TestDataStoreStatusProviderReturnsLatestStatus(t *testing.T) {
	factoryWithUpdater := dataStoreFactoryThatExposesUpdater{
		underlyingFactory: ldcomponents.PersistentDataStore(singlePersistentDataStoreFactory{sharedtest.NewMockPersistentDataStore()}),
	}
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		DataStore:  &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Loggers:    sharedtest.NewTestLoggers(),
	}
	client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	originalStatus := interfaces.DataStoreStatus{Available: true}
	newStatus := interfaces.DataStoreStatus{Available: false}

	assert.Equal(t, originalStatus, client.GetDataStoreStatusProvider().GetStatus())

	factoryWithUpdater.dataStoreUpdates.UpdateStatus(newStatus)

	assert.Equal(t, newStatus, client.GetDataStoreStatusProvider().GetStatus())
}

func TestDataStoreStatusProviderSendsStatusUpdates(t *testing.T) {
	factoryWithUpdater := dataStoreFactoryThatExposesUpdater{
		underlyingFactory: ldcomponents.PersistentDataStore(singlePersistentDataStoreFactory{sharedtest.NewMockPersistentDataStore()}),
	}
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		DataStore:  &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
	}
	client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
	require.NoError(t, err)
	defer client.Close()

	newStatus := interfaces.DataStoreStatus{Available: false}

	statusCh := client.GetDataStoreStatusProvider().AddStatusListener()

	factoryWithUpdater.dataStoreUpdates.UpdateStatus(newStatus)

	select {
	case s := <-statusCh:
		assert.Equal(t, newStatus, s)
	case <-time.After(time.Second * 2):
		assert.Fail(t, "timed out waiting for new status")
	}
}
