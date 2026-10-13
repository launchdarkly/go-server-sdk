package datastore

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	s "github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentDataStoreWrapperReleaseClauseValues(t *testing.T) {
	for _, release := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[release], func(t *testing.T) {
			// The wrapper does not cache, so each read deserializes the item from the core.
			wrapper := NewPersistentDataStoreWrapper(mocks.NewMockPersistentDataStore(),
				NewDataStoreUpdateSinkImpl(internal.NewBroadcaster[interfaces.DataStoreStatus]()),
				testUncached.ttl(), s.NewTestLoggers(), datakinds.NewDeserializeOptions(release))
			defer wrapper.Close()
			flag := s.FlagWithManyInClauseValues("flagkey", 1)
			segment := s.SegmentWithManyInClauseValues("segmentkey", 1)
			require.NoError(t, wrapper.Init(s.NewDataSetBuilder().Flags(flag).Segments(segment).Build()))

			var expected []ldvalue.Value
			if !release {
				expected = s.ManyInClauseValues()
			}

			flagItem, err := wrapper.Get(datakinds.Features, flag.Key)
			require.NoError(t, err)
			allFlags, err := wrapper.GetAll(datakinds.Features)
			require.NoError(t, err)
			require.Len(t, allFlags, 1)
			assert.Equal(t, expected, s.FirstClauseValues(flagItem.Item.(*ldmodel.FeatureFlag)))
			assert.Equal(t, expected, s.FirstClauseValues(allFlags[0].Item.Item.(*ldmodel.FeatureFlag)))

			segmentItem, err := wrapper.Get(datakinds.Segments, segment.Key)
			require.NoError(t, err)
			allSegments, err := wrapper.GetAll(datakinds.Segments)
			require.NoError(t, err)
			require.Len(t, allSegments, 1)
			assert.Equal(t, expected, s.FirstSegmentClauseValues(segmentItem.Item.(*ldmodel.Segment)))
			assert.Equal(t, expected, s.FirstSegmentClauseValues(allSegments[0].Item.Item.(*ldmodel.Segment)))

			assert.Equal(t, s.ManyInClauseValues(), s.FirstClauseValues(&flag), "original flag must not change")
			assert.Equal(t, s.ManyInClauseValues(), s.FirstSegmentClauseValues(&segment),
				"original segment must not change")
		})
	}
}
