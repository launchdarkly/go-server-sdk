package datasourcev2

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeReleaseClauseValuesContext returns a ClientContextImpl value, not a pointer, because the FDv2
// data system gives a value to its components.
func makeReleaseClauseValuesContext(releaseClauseValues bool) internal.ClientContextImpl {
	return internal.ClientContextImpl{
		BasicClientContext:  sharedtest.NewTestContext("", nil, nil),
		ReleaseClauseValues: releaseClauseValues,
	}
}

func makeReleaseData() *ldservicesv2.ServerSDKData {
	return ldservicesv2.NewServerSDKData().
		Flags(sharedtest.FlagWithManyInClauseValues("flagkey", 1)).
		Segments(sharedtest.SegmentWithManyInClauseValues("segmentkey", 1))
}

// assertChangeSetClauseValuesReleased checks that the flag and the segment of the changeset released
// their clause values if release is true, and kept them otherwise.
func assertChangeSetClauseValuesReleased(t *testing.T, release bool, changeSet *subsystems.ChangeSet) {
	t.Helper()
	require.NotNil(t, changeSet)
	collections, err := changeSet.Collections()
	require.NoError(t, err)

	var flag *ldmodel.FeatureFlag
	var segment *ldmodel.Segment
	for _, coll := range collections {
		require.Len(t, coll.Items, 1)
		switch coll.Kind {
		case datakinds.Features:
			flag = coll.Items[0].Item.Item.(*ldmodel.FeatureFlag)
		case datakinds.Segments:
			segment = coll.Items[0].Item.Item.(*ldmodel.Segment)
		}
	}
	require.NotNil(t, flag, "flag not found")
	require.NotNil(t, segment, "segment not found")

	if release {
		assert.Nil(t, sharedtest.FirstClauseValues(flag))
		assert.Nil(t, sharedtest.FirstSegmentClauseValues(segment))
	} else {
		assert.Equal(t, sharedtest.ManyInClauseValues(), sharedtest.FirstClauseValues(flag))
		assert.Equal(t, sharedtest.ManyInClauseValues(), sharedtest.FirstSegmentClauseValues(segment))
	}
}

func TestPollingRequesterReleaseClauseValues(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			payload := makeReleaseData().ToInitializerPayload(subsystems.NewSelector("state", 1))
			httphelpers.WithServer(ldservices.ServerSidePollingV2ServiceHandler(payload), func(ts *httptest.Server) {
				r := newPollingRequester(makeReleaseClauseValuesContext(release), nil, ts.URL, "")

				changeSet, _, err := r.Request(context.Background(), subsystems.NoSelector())

				require.NoError(t, err)
				assertChangeSetClauseValuesReleased(t, release, changeSet)
			})
		})
	}
}

func TestFDv1FallbackPollingReleaseClauseValues(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			flag := sharedtest.FlagWithManyInClauseValues("flagkey", 1)
			segment := sharedtest.SegmentWithManyInClauseValues("segmentkey", 1)
			data := ldservices.NewServerSDKData().Flags(&flag).Segments(&segment)
			httphelpers.WithServer(ldservices.ServerSidePollingServiceHandler(data), func(ts *httptest.Server) {
				p := NewFDv1PollingProcessor(makeReleaseClauseValuesContext(release),
					datasource.PollingConfig{BaseURI: ts.URL})

				changeSet, _, err := p.requester.Request(context.Background(), subsystems.NoSelector())

				require.NoError(t, err)
				assertChangeSetClauseValuesReleased(t, release, changeSet)
			})
		})
	}
}

func TestStreamProcessorReleaseClauseValues(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			protocol := ldservicesv2.NewStreamingProtocol().
				WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
					ID: "id", Target: 0, Code: subsystems.IntentTransferFull, Reason: "payload-missing",
				}}).
				WithPutObjects(makeReleaseData().ToPutObjects()).
				WithTransferred("state", 1)
			streamHandler, _ := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
			httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
				sp := NewStreamProcessor(makeReleaseClauseValuesContext(release),
					datasource.StreamConfig{URI: ts.URL, InitialReconnectDelay: 50 * time.Millisecond})
				defer sp.Close()

				result := <-sp.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))

				require.Equal(t, interfaces.DataSourceStateValid, result.State)
				assertChangeSetClauseValuesReleased(t, release, result.ChangeSet)
			})
		})
	}
}
