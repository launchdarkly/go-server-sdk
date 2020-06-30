package ldclient

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
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
	factoryWithUpdater := sharedtest.DataSourceFactoryThatExposesUpdater{
		UnderlyingFactory: sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: true},
		},
	}
	config := Config{
		DataSource: &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Logging:    sharedtest.TestLogging(),
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(client, factoryWithUpdater.DataSourceUpdates)
}

func TestFlagTracker(t *testing.T) {
	t.Run("sends flag change events", func(t *testing.T) {
		clientDataSourceStatusProviderTest(func(client *LDClient, updates interfaces.DataSourceUpdates) {
			flag1v1 := ldbuilders.NewFlagBuilder("flagkey").Version(1).Build()
			allData := []interfaces.StoreCollection{
				{Kind: interfaces.DataKindFeatures(), Items: []interfaces.StoreKeyedItemDescriptor{
					{Key: flag1v1.Key, Item: sharedtest.FlagDescriptor(flag1v1)},
				}},
				{Kind: interfaces.DataKindSegments(), Items: nil},
			}
			_ = updates.Init(allData)

			ch1 := client.GetFlagTracker().AddFlagChangeListener()
			ch2 := client.GetFlagTracker().AddFlagChangeListener()

			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch1)
			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch2)

			flag1v2 := ldbuilders.NewFlagBuilder(flag1v1.Key).Version(2).Build()
			_ = updates.Upsert(interfaces.DataKindFeatures(), flag1v1.Key, sharedtest.FlagDescriptor(flag1v2))

			sharedtest.ExpectFlagChangeEvents(t, ch1, flag1v1.Key)
			sharedtest.ExpectFlagChangeEvents(t, ch2, flag1v1.Key)

			client.GetFlagTracker().RemoveFlagChangeListener(ch1)
			flag1v3 := ldbuilders.NewFlagBuilder(flag1v1.Key).Version(3).Build()
			_ = updates.Upsert(interfaces.DataKindFeatures(), flag1v1.Key, sharedtest.FlagDescriptor(flag1v3))

			sharedtest.ExpectFlagChangeEvents(t, ch2, flag1v1.Key)
			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch1)
		})
	})

	t.Run("sends flag value change events", func(t *testing.T) {
		flagKey := "important-flag"
		user := lduser.NewUser("important-user")
		otherUser := lduser.NewUser("unimportant-user")
		alwaysFalseFlag := ldbuilders.NewFlagBuilder(flagKey).Version(1).
			Variations(ldvalue.Bool(false), ldvalue.Bool(true)).On(false).OffVariation(0).Build()

		clientDataSourceStatusProviderTest(func(client *LDClient, updates interfaces.DataSourceUpdates) {
			initialData := []interfaces.StoreCollection{
				{Kind: interfaces.DataKindFeatures(), Items: []interfaces.StoreKeyedItemDescriptor{
					{Key: alwaysFalseFlag.Key, Item: sharedtest.FlagDescriptor(alwaysFalseFlag)},
				}},
				{Kind: interfaces.DataKindSegments(), Items: nil},
			}
			_ = updates.Init(initialData)

			ch1 := client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch2 := client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch3 := client.GetFlagTracker().AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())
			client.GetFlagTracker().RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works

			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch1)
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch2)
			sharedtest.ExpectNoMoreFlagValueChangeEvents(t, ch3)

			// make the flag true for the first user only, and broadcast a flag change event
			flagIsTrueForMyUserOnly := ldbuilders.NewFlagBuilder(flagKey).Version(2).
				Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
				On(true).FallthroughVariation(0).
				AddTarget(1, user.GetKey()).
				Build()
			_ = updates.Upsert(interfaces.DataKindFeatures(), flagKey, sharedtest.FlagDescriptor(flagIsTrueForMyUserOnly))

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
	factoryWithUpdater := sharedtest.DataStoreFactoryThatExposesUpdater{
		UnderlyingFactory: ldcomponents.PersistentDataStore(
			sharedtest.SinglePersistentDataStoreFactory{Instance: sharedtest.NewMockPersistentDataStore()},
		),
	}
	config := Config{
		DataSource: ldcomponents.ExternalUpdatesOnly(),
		DataStore:  &factoryWithUpdater,
		Events:     ldcomponents.NoEvents(),
		Logging:    sharedtest.TestLogging(),
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(client, factoryWithUpdater.DataStoreUpdates)
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
