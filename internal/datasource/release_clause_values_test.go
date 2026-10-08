package datasource

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeReleaseClauseValuesContext(releaseClauseValues bool) *internal.ClientContextImpl {
	return &internal.ClientContextImpl{
		BasicClientContext:  sharedtest.NewTestContext("", nil, nil),
		ReleaseClauseValues: releaseClauseValues,
	}
}

// assertClauseValuesReleased checks that the flag and the segment of the collections released their
// clause values if release is true, and kept them otherwise.
func assertClauseValuesReleased(t *testing.T, release bool, collections []ldstoretypes.Collection) {
	t.Helper()
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
	assertItemClauseValuesReleased(t, release, flag, segment)
}

// assertItemClauseValuesReleased checks the flag and the segment, if they are not nil.
func assertItemClauseValuesReleased(t *testing.T, release bool, flag *ldmodel.FeatureFlag, segment *ldmodel.Segment) {
	t.Helper()
	var expected []ldvalue.Value
	if !release {
		expected = sharedtest.ManyInClauseValues()
	}
	if flag != nil {
		assert.Equal(t, expected, sharedtest.FirstClauseValues(flag))
	}
	if segment != nil {
		assert.Equal(t, expected, sharedtest.FirstSegmentClauseValues(segment))
	}
}

func makeReleaseData() *ldservices.ServerSDKData {
	flag := sharedtest.FlagWithManyInClauseValues("flagkey", 1)
	segment := sharedtest.SegmentWithManyInClauseValues("segmentkey", 1)
	return ldservices.NewServerSDKData().Flags(&flag).Segments(&segment)
}

func TestPollingRequesterReleaseClauseValues(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			httphelpers.WithServer(ldservices.ServerSidePollingServiceHandler(makeReleaseData()), func(ts *httptest.Server) {
				r := NewPollingRequester(makeReleaseClauseValuesContext(release), nil, ts.URL, "")

				collections, _, _, err := r.Request()

				require.NoError(t, err)
				assertClauseValuesReleased(t, release, collections)
			})
		})
	}
}

func TestStreamProcessorReleaseClauseValues(t *testing.T) {
	timeout := time.Second
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			streamHandler, stream := ldservices.ServerSideStreamingServiceHandler(makeReleaseData().ToPutEvent())
			defer stream.Close()

			httphelpers.WithServer(streamHandler, func(ts *httptest.Server) {
				withMockDataSourceUpdates(func(updates *mocks.MockDataSourceUpdates) {
					sp := NewStreamProcessor(makeReleaseClauseValuesContext(release), updates,
						StreamConfig{URI: ts.URL, InitialReconnectDelay: briefDelay})
					defer sp.Close()
					closeWhenReady := make(chan struct{})
					sp.Start(closeWhenReady)
					th.AssertChannelClosed(t, closeWhenReady, timeout, "timed out waiting for data source to start")

					assertClauseValuesReleased(t, release, updates.DataStore.WaitForNextInit(t, timeout))

					patchedFlag := sharedtest.FlagWithManyInClauseValues("flagkey", 2)
					patchData, err := json.Marshal(map[string]any{"path": "/flags/flagkey", "data": &patchedFlag})
					require.NoError(t, err)
					stream.Send(httphelpers.SSEEvent{Event: patchEvent, Data: string(patchData)})
					upserted := updates.DataStore.WaitForUpsert(t, datakinds.Features, "flagkey", 2, timeout)
					assertItemClauseValuesReleased(t, release, upserted.Item.Item.(*ldmodel.FeatureFlag), nil)

					patchedSegment := sharedtest.SegmentWithManyInClauseValues("segmentkey", 2)
					patchData, err = json.Marshal(map[string]any{"path": "/segments/segmentkey", "data": &patchedSegment})
					require.NoError(t, err)
					stream.Send(httphelpers.SSEEvent{Event: patchEvent, Data: string(patchData)})
					upserted = updates.DataStore.WaitForUpsert(t, datakinds.Segments, "segmentkey", 2, timeout)
					assertItemClauseValuesReleased(t, release, nil, upserted.Item.Item.(*ldmodel.Segment))
				})
			})
		})
	}
}
