package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func clientDataSourceStatusProviderTest(action func(*LDClient, interfaces.DataSourceUpdates)) {
	factoryWithUpdater := dataSourceFactoryThatExposesUpdater{
		underlyingFactory: singleDataSourceFactory{mockDataSource{Initialized: true, StartFn: startImmediately}},
	}
	config := Config{
		DataSource: &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(client, factoryWithUpdater.dataSourceUpdates)
}

func TestDataSourceStatusProvider(t *testing.T) {
	t.Run("returns latest status", func(t *testing.T) {
		timeBeforeStarting := time.Now()
		clientDataSourceStatusProviderTest(func(client *LDClient, updates interfaces.DataSourceUpdates) {
			initialStatus := client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateInitializing, initialStatus.State)
			assert.False(t, initialStatus.StateSince.Before(timeBeforeStarting))
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, initialStatus.LastError)

			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: 401,
				Time:       time.Now(),
			}
			updates.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)

			newStatus := client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateOff, newStatus.State)
			assert.False(t, newStatus.StateSince.Before(errorInfo.Time))
			assert.Equal(t, errorInfo, newStatus.LastError)
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		clientDataSourceStatusProviderTest(func(client *LDClient, updates interfaces.DataSourceUpdates) {
			statusCh := client.GetDataSourceStatusProvider().AddStatusListener()

			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: 401,
				Time:       time.Now(),
			}
			updates.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)

			newStatus := <-statusCh
			assert.Equal(t, interfaces.DataSourceStateOff, newStatus.State)
			assert.False(t, newStatus.StateSince.Before(errorInfo.Time))
			assert.Equal(t, errorInfo, newStatus.LastError)
		})
	})
}

func clientDataStoreStatusProviderTest(action func(*LDClient, interfaces.DataStoreUpdates)) {
	factoryWithUpdater := dataStoreFactoryThatExposesUpdater{
		underlyingFactory: ldcomponents.PersistentDataStore(singlePersistentDataStoreFactory{sharedtest.NewMockPersistentDataStore()}),
	}
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		DataStore:  &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(client, factoryWithUpdater.dataStoreUpdates)
}

func TestDataStoreStatusProvider(t *testing.T) {
	t.Run("returns latest status", func(t *testing.T) {
		clientDataStoreStatusProviderTest(func(client *LDClient, updates interfaces.DataStoreUpdates) {
			originalStatus := interfaces.DataStoreStatus{Available: true}
			newStatus := interfaces.DataStoreStatus{Available: false}

			assert.Equal(t, originalStatus, client.GetDataStoreStatusProvider().GetStatus())

			updates.UpdateStatus(newStatus)

			assert.Equal(t, newStatus, client.GetDataStoreStatusProvider().GetStatus())
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		clientDataStoreStatusProviderTest(func(client *LDClient, updates interfaces.DataStoreUpdates) {
			newStatus := interfaces.DataStoreStatus{Available: false}
			statusCh := client.GetDataStoreStatusProvider().AddStatusListener()

			updates.UpdateStatus(newStatus)

			select {
			case s := <-statusCh:
				assert.Equal(t, newStatus, s)
			case <-time.After(time.Second * 2):
				assert.Fail(t, "timed out waiting for new status")
			}
		})
	})
}
