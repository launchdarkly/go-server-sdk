package ldclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdatav2"

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
			// The value channel is closed by a forwarding goroutine, so allow time for it to run.
			th.AssertChannelClosed(t, ch2, time.Second)

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
			validStateReached := p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, 2*time.Second)
			assert.True(t, validStateReached, "timed out waiting for data source to reach VALID state")

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
			validStateReached := p.client.GetDataSourceStatusProvider().WaitFor(interfaces.DataSourceStateValid, 2*time.Second)
			assert.True(t, validStateReached, "timed out waiting for data source to reach VALID state")

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

// The tests below use a flag change listener that evaluates flags while it handles events, the way
// an application listener does. The client must keep delivering events to such a listener and
// keep serving evaluations, even when more events arrive than the listener's channel buffers.
// Every wait in these tests is bounded, so a client that stalls fails the test instead of hanging
// the test binary.

// listenerTestEventCount is more events than a listener channel buffers.
const listenerTestEventCount = 20

const listenerTestTimeout = time.Second * 2

var listenerTestUser = lduser.NewUser("listener-user")

func TestFlagChangeListenerCanEvaluateDuringUpdatesV2(t *testing.T) {
	t.Run("TestData updates flags one at a time", func(t *testing.T) {
		td := ldtestdatav2.DataSource()
		client := makeTestDataV2ListenerClient(t, td)
		handled := startEvaluatingListener(client, listenerTestEventCount, time.Millisecond*5)

		// Update can block once the client stops consuming changes, so it must not run on the
		// test goroutine.
		go func() {
			for i := 0; i < listenerTestEventCount; i++ {
				td.Update(td.Flag(fmt.Sprintf("flag-%d", i)).BooleanFlag().VariationForAll(true))
			}
		}()

		th.AssertChannelClosed(t, handled, listenerTestTimeout, "listener did not handle every flag change")
	})

	t.Run("a segment change affects many flags", func(t *testing.T) {
		segment := ldbuilders.NewSegmentBuilder("beta-users").Version(1).Build()
		data := ldservicesv2.NewServerSDKData().Segments(segment)
		for i := 0; i < listenerTestEventCount; i++ {
			data.Flags(segmentMatchFlag(fmt.Sprintf("flag-%d", i), segment.Key))
		}

		streamingListenerTestV2(t, data, nil, func(p clientListenersV2TestParams) {
			handled := startEvaluatingListener(p.client, listenerTestEventCount, 0)

			// One segment change produces a flag change event for every flag that references it.
			changed := ldbuilders.NewSegmentBuilder(segment.Key).Version(2).Included(listenerTestUser.Key()).Build()
			p.protocol.WithPutObjects(ldservicesv2.NewServerSDKData().Segments(changed).ToPutObjects()).
				WithTransferred("state", 2).
				Enqueue(p.control)

			th.AssertChannelClosed(t, handled, listenerTestTimeout, "listener did not handle every flag change")
		})
	})

	t.Run("the listener is registered before the first payload", func(t *testing.T) {
		data := ldservicesv2.NewServerSDKData()
		for i := 0; i < listenerTestEventCount; i++ {
			data.Flags(booleanFlagV2(fmt.Sprintf("flag-%d", i), 1, true))
		}
		firstPayload := make(chan struct{})

		streamingListenerTestV2(t, data, firstPayload, func(p clientListenersV2TestParams) {
			// Every flag in the first payload is new, so each one produces a flag change event.
			handled := startEvaluatingListener(p.client, listenerTestEventCount, 0)
			close(firstPayload)

			th.AssertChannelClosed(t, handled, listenerTestTimeout, "listener did not handle every flag change")
		})
	})

	t.Run("the stream delivers a burst of flag changes", func(t *testing.T) {
		data := ldservicesv2.NewServerSDKData()
		for i := 0; i < listenerTestEventCount; i++ {
			data.Flags(booleanFlagV2(fmt.Sprintf("flag-%d", i), 1, true))
		}

		streamingListenerTestV2(t, data, nil, func(p clientListenersV2TestParams) {
			handled := startEvaluatingListener(p.client, listenerTestEventCount, time.Millisecond*5)

			// Each change is its own transfer, as when many flags are edited in quick succession.
			for i := 0; i < listenerTestEventCount; i++ {
				changed := booleanFlagV2(fmt.Sprintf("flag-%d", i), 2, false)
				p.protocol.WithPutObjects(ldservicesv2.NewServerSDKData().Flags(changed).ToPutObjects()).
					WithTransferred("state", i+2)
			}
			p.protocol.Enqueue(p.control)

			th.AssertChannelClosed(t, handled, listenerTestTimeout, "listener did not handle every flag change")
		})
	})
}

func TestBusyFlagChangeListenerDoesNotBlockEvaluationsV2(t *testing.T) {
	td := ldtestdatav2.DataSource()
	td.Update(td.Flag("flag-0").BooleanFlag().VariationForAll(true))
	client := makeTestDataV2ListenerClient(t, td)

	// The listener is busy with its first event, as if it were waiting on a slow webhook, while
	// more changes arrive than its channel buffers.
	ch := client.GetFlagTracker().AddFlagChangeListener()
	release := make(chan struct{})
	defer close(release)
	go func() {
		<-ch
		<-release
		for range ch {
		}
	}()
	go func() {
		for i := 1; i <= listenerTestEventCount; i++ {
			td.Update(td.Flag(fmt.Sprintf("flag-%d", i)).BooleanFlag().VariationForAll(true))
		}
	}()
	// Give the changes time to fill the listener's channel.
	time.Sleep(time.Millisecond * 100)

	evaluated := make(chan struct{})
	go func() {
		_, _ = client.BoolVariation("flag-0", listenerTestUser, false)
		close(evaluated)
	}()
	th.AssertChannelClosed(t, evaluated, listenerTestTimeout, "evaluation waited for a busy flag change listener")
}

// startEvaluatingListener registers a flag change listener that evaluates each changed flag and then
// spends work handling the result. The returned channel is closed once the listener has handled
// eventCount events.
func startEvaluatingListener(client *LDClient, eventCount int, work time.Duration) <-chan struct{} {
	ch := client.GetFlagTracker().AddFlagChangeListener()
	handled := make(chan struct{})
	go func() {
		count := 0
		for event := range ch {
			_, _ = client.BoolVariation(event.Key, listenerTestUser, false)
			time.Sleep(work)
			count++
			if count == eventCount {
				close(handled)
			}
		}
	}()
	return handled
}

func makeTestDataV2ListenerClient(t *testing.T, td *ldtestdatav2.TestDataSynchronizer) *LDClient {
	config := Config{
		DataSystem: ldcomponents.DataSystem().Custom().Synchronizers(td),
		Events:     ldcomponents.NoEvents(),
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	client, err := MakeCustomClient(testSdkKey, config, time.Second*5)
	require.NoError(t, err)
	t.Cleanup(func() { closeClientWithinTimeout(t, client) })
	return client
}

// streamingListenerTestV2 is like clientListenersV2Test, but it serves the given initial data and
// closes the client with a bound. If firstPayload is not nil, the stream holds the initial payload
// until firstPayload is closed, and the client starts without waiting for it.
func streamingListenerTestV2(
	t *testing.T,
	initialData *ldservicesv2.ServerSDKData,
	firstPayload <-chan struct{},
	action func(clientListenersV2TestParams),
) {
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
			ID: "something-id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
		}}).
		WithPutObjects(initialData.ToPutObjects()).
		WithTransferred("state", 1)
	streamHandler, control := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	handler := streamHandler
	waitFor := time.Second * 5
	if firstPayload != nil {
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-firstPayload:
				streamHandler.ServeHTTP(w, r)
			case <-r.Context().Done():
			}
		})
		waitFor = 0
	}

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Streaming(),
		}
		client, err := MakeCustomClient(testSdkKey, config, waitFor)
		if waitFor > 0 {
			require.NoError(t, err)
		}
		defer closeClientWithinTimeout(t, client)
		action(clientListenersV2TestParams{client, protocol, control})
	})
}

// closeClientWithinTimeout fails the test instead of hanging when Close does not return.
func closeClientWithinTimeout(t *testing.T, client *LDClient) {
	closed := make(chan struct{})
	go func() {
		_ = client.Close()
		close(closed)
	}()
	th.AssertChannelClosed(t, closed, listenerTestTimeout, "client.Close did not return")
}

func booleanFlagV2(key string, version int, value bool) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(version).SingleVariation(ldvalue.Bool(value)).Build()
}

func segmentMatchFlag(key, segmentKey string) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).Version(1).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		AddRule(ldbuilders.NewRuleBuilder().ID("rule").Variation(1).
			Clauses(ldbuilders.SegmentMatchClause(segmentKey))).
		FallthroughVariation(0).OffVariation(0).Build()
}
