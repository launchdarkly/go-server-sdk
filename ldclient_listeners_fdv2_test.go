package ldclient

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

// This file contains tests for all of the event broadcaster/listener functionality in the client, plus
// related methods for looking at the same kinds of status values that can be broadcast to listeners.
// It uses mock implementations of the data source and data store, so that it is only the status
// monitoring mechanisms that are being tested, not the status behavior of specific real components.
//
// Parts of this functionality are also covered by lower-level component tests like
// DataSourceUpdateSinkImplTest. However, the tests here verify that the client is wiring the components
// together correctly so that they work from an application's point of view.

type clientListenersV2TestParams struct {
	client   *LDClient
	protocol *ldservicesv2.StreamingProtocol
	control  httphelpers.SSEStreamControl
}

func clientListenersV2Test(action func(clientListenersV2TestParams), handlers ...http.Handler) {
	clientListenersV2TestWithConfig(nil, action, handlers...)
}

func clientListenersV2TestWithConfig(configAction func(*Config), action func(clientListenersV2TestParams), handlers ...http.Handler) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "something-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred("state", 1)
	streamHandler, control := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	handler := httphelpers.SequentialHandler(streamHandler, handlers...)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Streaming(),
		}
		if configAction != nil {
			configAction(&config)
		}

		client, _ := MakeCustomClient(testSdkKey, config, time.Second*5)
		defer client.Close()
		action(clientListenersV2TestParams{client, protocol, control})
	})
}

func TestFlagTrackerV2(t *testing.T) {
	timeout := time.Millisecond * 100

	t.Run("sends flag change events", func(t *testing.T) {
		clientListenersV2Test(func(p clientListenersV2TestParams) {
			ch1 := p.client.GetFlagTracker().AddFlagChangeListener()
			ch2 := p.client.GetFlagTracker().AddFlagChangeListener()

			th.AssertNoMoreValues(t, ch1, timeout)
			th.AssertNoMoreValues(t, ch2, timeout)

			jsonFlag, _ := json.Marshal(alwaysFalseFlag)
			p.protocol.WithPutObject(subsystems.PutObject{
				Version: 10,
				Kind:    subsystems.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred("state", 2)
			p.protocol.Enqueue(p.control)

			sharedtest.ExpectFlagChangeEvents(t, ch1, alwaysTrueFlag.Key)
			sharedtest.ExpectFlagChangeEvents(t, ch2, alwaysTrueFlag.Key)

			p.client.GetFlagTracker().RemoveFlagChangeListener(ch1)
			th.AssertChannelClosed(t, ch1, time.Millisecond)

			jsonFlag, _ = json.Marshal(alwaysTrueFlag)
			p.protocol.WithPutObject(subsystems.PutObject{
				Version: 10,
				Kind:    subsystems.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred("state", 2)
			p.protocol.Enqueue(p.control)

			sharedtest.ExpectFlagChangeEvents(t, ch2, alwaysTrueFlag.Key)
		})
	})

	t.Run("sends flag value change events", func(t *testing.T) {
		user := lduser.NewUser("important-user")
		otherUser := lduser.NewUser("unimportant-user")

		clientListenersV2Test(func(p clientListenersV2TestParams) {
			ch1 := p.client.GetFlagTracker().AddFlagValueChangeListener(alwaysTrueFlag.Key, user, ldvalue.Null())
			ch2 := p.client.GetFlagTracker().AddFlagValueChangeListener(alwaysTrueFlag.Key, user, ldvalue.Null())
			ch3 := p.client.GetFlagTracker().AddFlagValueChangeListener(alwaysTrueFlag.Key, otherUser, ldvalue.Null())

			p.client.GetFlagTracker().RemoveFlagValueChangeListener(ch2) // just verifying that the remove method works
			th.AssertChannelClosed(t, ch2, time.Millisecond)

			th.AssertNoMoreValues(t, ch1, timeout)
			th.AssertNoMoreValues(t, ch3, timeout)

			jsonFlag, _ := json.Marshal(onlyTrueForImportantUsers)
			p.protocol.WithPutObject(subsystems.PutObject{
				Version: 10,
				Kind:    subsystems.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred("state", 1)
			p.protocol.Enqueue(p.control)

			// ch1 doesn't receive one, because the flag's value hasn't changed for user
			th.AssertNoMoreValues(t, ch1, timeout)

			// ch3 receives a value change event
			event1 := <-ch3
			assert.Equal(t, alwaysTrueFlag.Key, event1.Key)
			assert.Equal(t, ldvalue.Bool(true), event1.OldValue)
			assert.Equal(t, ldvalue.Bool(false), event1.NewValue)
			th.AssertNoMoreValues(t, ch3, timeout)
		})
	})
}

func TestDataSourceStatusProviderV2(t *testing.T) {
	t.Run("returns latest status", func(t *testing.T) {
		timeBeforeStarting := time.Now()
		clientListenersV2Test(func(p clientListenersV2TestParams) {
			initialStatus := p.client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateValid, initialStatus.State)
			assert.False(t, initialStatus.StateSince.Before(timeBeforeStarting))
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, initialStatus.LastError)

			p.control.Close()

			timeout := time.NewTimer(time.Second)
			for {
				select {
				case <-timeout.C:
					assert.Fail(t, "timed out waiting for new status")
					return
				default:
					status := p.client.GetDataSourceStatusProvider().GetStatus()
					if status.State == interfaces.DataSourceStateOff {
						return
					}
				}
			}
		}, httphelpers.HandlerWithStatus(401))
	})

	t.Run("sends latest status", func(t *testing.T) {
		timeBeforeStarting := time.Now()
		clientListenersV2Test(func(p clientListenersV2TestParams) {
			initialStatus := p.client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateValid, initialStatus.State)
			assert.False(t, initialStatus.StateSince.Before(timeBeforeStarting))
			assert.Equal(t, interfaces.DataSourceErrorInfo{}, initialStatus.LastError)

			ch := p.client.GetDataSourceStatusProvider().AddStatusListener()
			p.control.Close()

			status := <-ch
			assert.Equal(t, interfaces.DataSourceStateInterrupted, status.State)
			assert.Equal(t, interfaces.DataSourceErrorKindNetworkError, status.LastError.Kind)
			assert.Equal(t, 0, status.LastError.StatusCode)

			status = <-ch
			assert.Equal(t, interfaces.DataSourceStateInterrupted, status.State)
			assert.Equal(t, interfaces.DataSourceErrorKindErrorResponse, status.LastError.Kind)
			assert.Equal(t, 401, status.LastError.StatusCode)

			status = <-ch
			assert.Equal(t, interfaces.DataSourceStateOff, status.State)

			status = p.client.GetDataSourceStatusProvider().GetStatus()
			assert.Equal(t, interfaces.DataSourceStateOff, status.State)
		}, httphelpers.HandlerWithStatus(401))
	})

	t.Run("waitFor detects correct status", func(t *testing.T) {
		clientListenersV2Test(func(p clientListenersV2TestParams) {
			// Can wait for the valid state
			foundIt := p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Second)
			assert.True(t, foundIt)

			// Negative timeouts fire immediately
			start := time.Now()
			foundIt = p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateInterrupted, time.Second*-1)
			assert.WithinDuration(t, time.Now(), start, time.Millisecond*30)
			assert.False(t, foundIt)

			// Make sure timeout will occur when a state cannot be found
			foundIt = p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateInterrupted, time.Second)
			assert.False(t, foundIt)

			// Shut it down and make sure it's stopped.
			p.control.Close()
			foundIt = p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateOff, time.Second)
			assert.True(t, foundIt)

			// Ensure that an off status doesn't require the use of a timer.
			start = time.Now()
			foundIt = p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, time.Hour*100)
			assert.WithinDuration(t, time.Now(), start, time.Millisecond*30)
			assert.False(t, foundIt)
		}, httphelpers.HandlerWithStatus(401))
	})
}

func TestBigSegmentsStoreStatusProviderV2(t *testing.T) {
	t.Run("returns unavailable status when not configured", func(t *testing.T) {
		clientListenersV2Test(func(p clientListenersV2TestParams) {
			assert.Equal(t, interfaces.BigSegmentStoreStatus{},
				p.client.GetBigSegmentStoreStatusProvider().GetStatus())
		})
	})

	t.Run("sends status updates", func(t *testing.T) {
		store := &mocks.MockBigSegmentStore{}
		store.TestSetMetadataToCurrentTime()
		storeFactory := mocks.SingleComponentConfigurer[subsystems.BigSegmentStore]{Instance: store}
		clientListenersV2TestWithConfig(
			func(c *Config) {
				c.BigSegments = ldcomponents.BigSegments(storeFactory).StatusPollInterval(time.Millisecond * 10)
			},
			func(p clientListenersV2TestParams) {
				statusCh := p.client.GetBigSegmentStoreStatusProvider().AddStatusListener()

				mocks.ExpectBigSegmentStoreStatus(
					t,
					statusCh,
					p.client.GetBigSegmentStoreStatusProvider().GetStatus,
					time.Second,
					interfaces.BigSegmentStoreStatus{Available: true},
				)

				store.TestSetMetadataState(subsystems.BigSegmentStoreMetadata{}, errors.New("failing"))

				mocks.ExpectBigSegmentStoreStatus(
					t,
					statusCh,
					p.client.GetBigSegmentStoreStatusProvider().GetStatus,
					time.Second,
					interfaces.BigSegmentStoreStatus{Available: false},
				)
			})
	})
}
