package ldclient

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers/ldtestdata"
)

// This file contains tests for all of the event broadcaster/listener functionality in the client, plus
// related methods for looking at the same kinds of status values that can be broadcast to listeners.
// It uses mock implementations of the data source and data store, so that it is only the status
// monitoring mechanisms that are being tested, not the status behavior of specific real components.
//
// Parts of this functionality are also covered by lower-level component tests like
// DataSourceUpdatesImplTest. However, the tests here verify that the client is wiring the components
// together correctly so that they work from an application's point of view.

type clientListenersTestParams struct {
	client           *LDClient
	testData         *ldtestdata.TestDataSource
	dataStoreUpdates interfaces.DataStoreUpdates
}

func clientListenersTest(action func(clientListenersTestParams)) {
	testData := ldtestdata.DataSource()
	factoryWithUpdater := &sharedtest.DataStoreFactoryThatExposesUpdater{
		UnderlyingFactory: ldcomponents.PersistentDataStore(
			sharedtest.SinglePersistentDataStoreFactory{Instance: sharedtest.NewMockPersistentDataStore()},
		),
	}
	config := Config{
		DataSource: testData,
		DataStore:  factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Logging:    sharedtest.TestLogging(),
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(clientListenersTestParams{client, testData, factoryWithUpdater.DataStoreUpdates})
}

func TestFlagTracker(t *testing.T) {
	flagKey := "important-flag"

	t.Run("sends flag change events", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			p.testData.Update(p.testData.Flag(flagKey))

			ch1 := p.client.GetFlagTracker().AddFlagChangeListener()
			ch2 := p.client.GetFlagTracker().AddFlagChangeListener()

			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch1)
			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch2)

			p.testData.Update(p.testData.Flag(flagKey))

			sharedtest.ExpectFlagChangeEvents(t, ch1, flagKey)
			sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)

			p.client.GetFlagTracker().RemoveFlagChangeListener(ch1)
			p.testData.Update(p.testData.Flag(flagKey))

			sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)
			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch1)
		})
	})

	t.Run("sends flag value change events", func(t *testing.T) {
		flagKey := "important-flag"
		user := lduser.NewUser("important-user")
		otherUser := lduser.NewUser("unimportant-user")

		clientListenersTest(func(p clientListenersTestParams) {
			p.testData.Update(p.testData.Flag(flagKey).VariationForAllUsers(false))

			ch1 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch2 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch3 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())
			p.client.GetFlagTracker().RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works

			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch1)
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)

			// make the flag true for the first user only, and broadcast a flag change event
			p.testData.Update(p.testData.Flag(flagKey).VariationForUser(user.GetKey(), true))

			// ch1 receives a value change event
			event1 := <-ch1
			assert.Equal(t, flagKey, event1.Key)
			assert.Equal(t, ldvalue.Bool(false), event1.OldValue)
			assert.Equal(t, ldvalue.Bool(true), event1.NewValue)

			// ch2 doesn't receive one, because it was unregistered
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)

			// ch3 doesn't receive one, because the flag's value hasn't changed for otherUser
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)
		})
	})
}

func TestDataSourceStatusProvider(t *testing.T) {
	t.Run("returns latest status", func(t *testing.T) {
		timeBeforeStarting := time.Now()
		clientListenersTest(func(p clientListenersTestParams) {
			initialStatus := p.client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateValid, initialStatus.State)
			assert.False(t, initialStatus.StateSince.Before(timeBeforeStarting))
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, initialStatus.LastError)

			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: 401,
				Time:       time.Now(),
			}
			p.testData.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)

			newStatus := p.client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateOff, newStatus.State)
			assert.False(t, newStatus.StateSince.Before(errorInfo.Time))
			assert.Equal(t, errorInfo, newStatus.LastError)
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			statusCh := p.client.GetDataSourceStatusProvider().AddStatusListener()

			errorInfo := interfaces.DataSourceErrorInfo{
				Kind:       interfaces.DataSourceErrorKindErrorResponse,
				StatusCode: 401,
				Time:       time.Now(),
			}
			p.testData.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)

			newStatus := <-statusCh
			assert.Equal(t, interfaces.DataSourceStateOff, newStatus.State)
			assert.False(t, newStatus.StateSince.Before(errorInfo.Time))
			assert.Equal(t, errorInfo, newStatus.LastError)
		})
	})
}

func TestDataStoreStatusProvider(t *testing.T) {
	t.Run("returns latest status", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			originalStatus := interfaces.DataStoreStatus{Available: true}
			newStatus := interfaces.DataStoreStatus{Available: false}

			assert.Equal(t, originalStatus, p.client.GetDataStoreStatusProvider().GetStatus())

			p.dataStoreUpdates.UpdateStatus(newStatus)

			assert.Equal(t, newStatus, p.client.GetDataStoreStatusProvider().GetStatus())
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			newStatus := interfaces.DataStoreStatus{Available: false}
			statusCh := p.client.GetDataStoreStatusProvider().AddStatusListener()

			p.dataStoreUpdates.UpdateStatus(newStatus)

			select {
			case s := <-statusCh:
				assert.Equal(t, newStatus, s)
			case <-time.After(time.Second * 2):
				assert.Fail(t, "timed out waiting for new status")
			}
		})
	})
}
