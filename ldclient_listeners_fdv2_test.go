package ldclient

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
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

func clientListenersV2Test(action func(clientListenersV2TestParams)) {
	clientListenersV2TestWithConfig(nil, action)
}

func clientListenersV2TestWithConfig(configAction func(*Config), action func(clientListenersV2TestParams)) {
	data := ldservicesv2.NewServerSDKData().Flags(alwaysTrueFlag)
	protocol := ldservicesv2.NewStreamingProtocol().
		WithIntent(fdv2proto.ServerIntent{Payload: fdv2proto.Payload{
			ID: "something-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
		}}).
		WithPutObjects(data.ToPutObjects()).
		WithTransferred(1)
	streamHandler, control := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)

	handler := httphelpers.SequentialHandler(streamHandler)

	httphelpers.WithServer(handler, func(server *httptest.Server) {
		logCapture := ldlogtest.NewMockLog()
		defer logCapture.Dump(os.Stdout)

		config := Config{
			Events:     ldcomponents.NoEvents(),
			Logging:    ldcomponents.Logging().Loggers(logCapture.Loggers),
			DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default(),
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
			p.protocol.WithPutObject(fdv2proto.PutObject{
				Version: 10,
				Kind:    fdv2proto.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred(1)
			p.protocol.Enqueue(p.control)

			sharedtest.ExpectFlagChangeEvents(t, ch1, alwaysTrueFlag.Key)
			sharedtest.ExpectFlagChangeEvents(t, ch2, alwaysTrueFlag.Key)

			p.client.GetFlagTracker().RemoveFlagChangeListener(ch1)
			th.AssertChannelClosed(t, ch1, time.Millisecond)

			jsonFlag, _ = json.Marshal(alwaysTrueFlag)
			p.protocol.WithPutObject(fdv2proto.PutObject{
				Version: 10,
				Kind:    fdv2proto.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred(2)
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
			p.protocol.WithPutObject(fdv2proto.PutObject{
				Version: 10,
				Kind:    fdv2proto.FlagKind,
				Key:     alwaysTrueFlag.Key,
				Object:  jsonFlag,
			})
			p.protocol.WithTransferred(1)
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
