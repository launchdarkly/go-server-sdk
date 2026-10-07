package ldclient

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releaseFlag and releaseSegment have an "in" clause with enough values that the SDK releases its Values
// list if Config.ReleaseClauseValues is true.
var (
	releaseFlag    = sharedtest.FlagWithManyInClauseValues("flagkey", 1)       //nolint:gochecknoglobals
	releaseSegment = sharedtest.SegmentWithManyInClauseValues("segmentkey", 1) //nolint:gochecknoglobals
)

// assertClientReleasedClauseValues checks that the stored flag and segment released their clause values
// if release is true, and kept them otherwise. It also checks that the flag still evaluates correctly.
func assertClientReleasedClauseValues(t *testing.T, client *LDClient, release bool) {
	t.Helper()
	flagItem, err := client.dataSystem.Store().Get(datakinds.Features, releaseFlag.Key)
	require.NoError(t, err)
	require.NotNil(t, flagItem.Item, "flag not found")
	segmentItem, err := client.dataSystem.Store().Get(datakinds.Segments, releaseSegment.Key)
	require.NoError(t, err)
	require.NotNil(t, segmentItem.Item, "segment not found")

	flagValues := sharedtest.FirstClauseValues(flagItem.Item.(*ldmodel.FeatureFlag))
	segmentValues := sharedtest.FirstSegmentClauseValues(segmentItem.Item.(*ldmodel.Segment))
	if release {
		assert.Nil(t, flagValues)
		assert.Nil(t, segmentValues)
	} else {
		assert.Equal(t, sharedtest.ManyInClauseValues(), flagValues)
		assert.Equal(t, sharedtest.ManyInClauseValues(), segmentValues)
	}

	for _, v := range sharedtest.ManyInClauseValues() {
		value, err := client.BoolVariation(releaseFlag.Key, ldcontext.New(v.StringValue()), false)
		require.NoError(t, err)
		assert.True(t, value, "context key: %s", v)
	}
	value, err := client.BoolVariation(releaseFlag.Key, ldcontext.New("other"), true)
	require.NoError(t, err)
	assert.False(t, value)
}

// runReleaseClauseValuesTest makes a client with the configuration, once with ReleaseClauseValues true
// and once with it false, and checks the stored items. Each run calls makeConfig, so that each client can
// get its own server.
func runReleaseClauseValuesTest(t *testing.T, makeConfig func(t *testing.T) Config) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			logCapture := ldlogtest.NewMockLog()
			defer logCapture.DumpIfTestFailed(t)
			config := makeConfig(t)
			config.Events = ldcomponents.NoEvents()
			config.Logging = ldcomponents.Logging().Loggers(logCapture.Loggers)
			config.ReleaseClauseValues = release

			client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
			require.NoError(t, err)
			defer client.Close()

			assertClientReleasedClauseValues(t, client, release)
		})
	}
}

func fdv1ReleaseData() *ldservices.ServerSDKData {
	return ldservices.NewServerSDKData().Flags(&releaseFlag).Segments(&releaseSegment)
}

func fdv2ReleaseData() *ldservicesv2.ServerSDKData {
	return ldservicesv2.NewServerSDKData().Flags(releaseFlag).Segments(releaseSegment)
}

// startServer starts a test server that stops when the test ends.
func startServer(t *testing.T, handler http.Handler) *httptest.Server {
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

// makePersistentStoreWithReleaseData returns a persistent store that already holds the serialized flag and
// segment, as if another process wrote them.
func makePersistentStoreWithReleaseData(t *testing.T) *mocks.MockPersistentDataStore {
	core := mocks.NewMockPersistentDataStore()
	require.NoError(t, core.Init([]ldstoretypes.SerializedCollection{
		{Kind: datakinds.Features, Items: []ldstoretypes.KeyedSerializedItemDescriptor{{
			Key: releaseFlag.Key,
			Item: ldstoretypes.SerializedItemDescriptor{Version: 1,
				SerializedItem: datakinds.Features.Serialize(sharedtest.FlagDescriptor(releaseFlag))},
		}}},
		{Kind: datakinds.Segments, Items: []ldstoretypes.KeyedSerializedItemDescriptor{{
			Key: releaseSegment.Key,
			Item: ldstoretypes.SerializedItemDescriptor{Version: 1,
				SerializedItem: datakinds.Segments.Serialize(sharedtest.SegmentDescriptor(releaseSegment))},
		}}},
	}))
	return core
}

func TestClientReleaseClauseValuesWithFDv1Streaming(t *testing.T) {
	runReleaseClauseValuesTest(t, func(t *testing.T) Config {
		streamHandler, stream := ldservices.ServerSideStreamingServiceHandler(fdv1ReleaseData().ToPutEvent())
		server := startServer(t, streamHandler)
		// Cleanups run in reverse order. The stream must end before the server can close.
		t.Cleanup(func() { _ = stream.Close() })
		return Config{ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL}}
	})
}

func TestClientReleaseClauseValuesWithFDv1Polling(t *testing.T) {
	runReleaseClauseValuesTest(t, func(t *testing.T) Config {
		server := startServer(t, ldservices.ServerSidePollingServiceHandler(fdv1ReleaseData()))
		return Config{
			DataSource:       ldcomponents.PollingDataSource(),
			ServiceEndpoints: interfaces.ServiceEndpoints{Polling: server.URL},
		}
	})
}

func TestClientReleaseClauseValuesWithFDv1PersistentStore(t *testing.T) {
	for name, ttl := range map[string]time.Duration{"uncached": 0, "cached": 30 * time.Second, "cache forever": -1} {
		t.Run(name, func(t *testing.T) {
			runReleaseClauseValuesTest(t, func(t *testing.T) Config {
				return Config{
					DataSource: ldcomponents.ExternalUpdatesOnly(),
					DataStore: ldcomponents.PersistentDataStore(
						mocks.SingleComponentConfigurer[subsystems.PersistentDataStore]{
							Instance: makePersistentStoreWithReleaseData(t),
						}).CacheTime(ttl),
				}
			})
		})
	}
}

func TestClientReleaseClauseValuesWithFDv2Streaming(t *testing.T) {
	runReleaseClauseValuesTest(t, func(t *testing.T) Config {
		protocol := ldservicesv2.NewStreamingProtocol().
			WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
				ID: "id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
			}}).
			WithPutObjects(fdv2ReleaseData().ToPutObjects()).
			WithTransferred("state", 1)
		streamHandler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
		// The polling initializer gets an error, so that the streaming synchronizer provides the data.
		server := startServer(t, httphelpers.SequentialHandler(httphelpers.HandlerWithStatus(500), streamHandler))
		// Cleanups run in reverse order. The stream must end before the server can close.
		t.Cleanup(func() { _ = stream.Close() })
		return Config{DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default()}
	})
}

func TestClientReleaseClauseValuesWithFDv2Polling(t *testing.T) {
	runReleaseClauseValuesTest(t, func(t *testing.T) Config {
		pollHandler := ldservices.ServerSidePollingV2ServiceHandler(
			fdv2ReleaseData().ToInitializerPayload(subsystems.NewSelector("state", 1)))
		// The polling initializer provides the data. The streaming synchronizer then gets an error, so
		// that it does not change the data.
		server := startServer(t, httphelpers.SequentialHandler(pollHandler, httphelpers.HandlerWithStatus(503)))
		return Config{DataSystem: ldcomponents.DataSystem().WithRelayProxyEndpoints(server.URL).Default()}
	})
}

func TestClientReleaseClauseValuesWithFDv2PersistentStore(t *testing.T) {
	for name, ttl := range map[string]time.Duration{"uncached": 0, "cached": 30 * time.Second} {
		t.Run(name, func(t *testing.T) {
			runReleaseClauseValuesTest(t, func(t *testing.T) Config {
				return Config{
					DataSystem: ldcomponents.DataSystem().Daemon(ldcomponents.PersistentDataStore(
						mocks.SingleComponentConfigurer[subsystems.PersistentDataStore]{
							Instance: makePersistentStoreWithReleaseData(t),
						}).CacheTime(ttl)),
				}
			})
		})
	}
}
