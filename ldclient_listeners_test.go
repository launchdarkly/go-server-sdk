package ldclient

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/testhelpers/ldtestdata"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
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
	dataStoreUpdates subsystems.DataStoreUpdates
}

func clientListenersTest(action func(clientListenersTestParams)) {
	clientListenersTestWithConfig(nil, action)
}

func clientListenersTestWithConfig(configAction func(*Config), action func(clientListenersTestParams)) {
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
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	if configAction != nil {
		configAction(&config)
	}
	client, _ := MakeCustomClient(testSdkKey, config, 5*time.Second)
	defer client.Close()
	action(clientListenersTestParams{client, testData, factoryWithUpdater.DataStoreUpdates})
}

func TestFlagTracker(t *testing.T) {
	flagKey := "important-flag"
	timeout := time.Millisecond * 100

	t.Run("sends flag change events", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			p.testData.Update(p.testData.Flag(flagKey))

			ch1 := p.client.GetFlagTracker().AddFlagChangeListener()
			ch2 := p.client.GetFlagTracker().AddFlagChangeListener()

			th.AssertNoMoreValues(t, ch1, timeout)
			th.AssertNoMoreValues(t, ch2, timeout)

			p.testData.Update(p.testData.Flag(flagKey))

			sharedtest.ExpectFlagChangeEvents(t, ch1, flagKey)
			sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)

			p.client.GetFlagTracker().RemoveFlagChangeListener(ch1)
			th.AssertChannelClosed(t, ch1, time.Millisecond)

			p.testData.Update(p.testData.Flag(flagKey))

			sharedtest.ExpectFlagChangeEvents(t, ch2, flagKey)
		})
	})

	t.Run("sends flag value change events", func(t *testing.T) {
		flagKey := "important-flag"
		user := lduser.NewUser("important-user")
		otherUser := lduser.NewUser("unimportant-user")

		clientListenersTest(func(p clientListenersTestParams) {
			p.testData.Update(p.testData.Flag(flagKey).VariationForAll(false))

			ch1 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch2 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, user, ldvalue.Null())
			ch3 := p.client.GetFlagTracker().AddFlagValueChangeListener(flagKey, otherUser, ldvalue.Null())

			p.client.GetFlagTracker().RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works
			th.AssertChannelClosed(t, ch2, time.Millisecond)

			th.AssertNoMoreValues(t, ch1, timeout)
			th.AssertNoMoreValues(t, ch3, timeout)

			// make the flag true for the first user only, and broadcast a flag change event
			p.testData.Update(p.testData.Flag(flagKey).VariationForUser(user.Key(), true))

			// ch1 receives a value change event
			event1 := <-ch1
			assert.Equal(t, flagKey, event1.Key)
			assert.Equal(t, ldvalue.Bool(false), event1.OldValue)
			assert.Equal(t, ldvalue.Bool(true), event1.NewValue)

			// ch3 doesn't receive one, because the flag's value hasn't changed for otherUser
			th.AssertNoMoreValues(t, ch3, timeout)
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

			s := th.RequireValue(t, statusCh, time.Second*2, "timed out waiting for new status")
			assert.Equal(t, newStatus, s)
		})
	})
}

func TestBigSegmentsStoreStatusProvider(t *testing.T) {
	t.Run("returns unavailable status when not configured", func(t *testing.T) {
		clientListenersTest(func(p clientListenersTestParams) {
			assert.Equal(t, interfaces.BigSegmentStoreStatus{},
				p.client.GetBigSegmentStoreStatusProvider().GetStatus())
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		store := &sharedtest.MockBigSegmentStore{}
		store.TestSetMetadataToCurrentTime()
		storeFactory := sharedtest.SingleBigSegmentStoreFactory{Store: store}
		clientListenersTestWithConfig(
			func(c *Config) {
				c.BigSegments = ldcomponents.BigSegments(storeFactory).StatusPollInterval(time.Millisecond * 10)
			},
			func(p clientListenersTestParams) {
				statusCh := p.client.GetBigSegmentStoreStatusProvider().AddStatusListener()

				sharedtest.ExpectBigSegmentStoreStatus(
					t,
					statusCh,
					p.client.GetBigSegmentStoreStatusProvider().GetStatus,
					time.Second,
					interfaces.BigSegmentStoreStatus{Available: true},
				)

				store.TestSetMetadataState(subsystems.BigSegmentStoreMetadata{}, errors.New("failing"))

				sharedtest.ExpectBigSegmentStoreStatus(
					t,
					statusCh,
					p.client.GetBigSegmentStoreStatusProvider().GetStatus,
					time.Second,
					interfaces.BigSegmentStoreStatus{Available: false},
				)
			})
	})
}
