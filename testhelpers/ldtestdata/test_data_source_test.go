package ldtestdata

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v2/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v6/testhelpers/ldservices"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	threeStringValues = []ldvalue.Value{ldvalue.String("red"), ldvalue.String("green"), ldvalue.String("blue")}
)

type testDataSourceTestParams struct {
	td      *TestDataSource
	updates *mocks.MockDataSourceUpdates
}

func testDataSourceTest(t *testing.T, action func(testDataSourceTestParams)) {
	t.Helper()
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	var p testDataSourceTestParams
	p.td = DataSource()
	p.updates = mocks.NewMockDataSourceUpdates(store)
	action(p)
}

func (p testDataSourceTestParams) withDataSource(t *testing.T, action func(subsystems.DataSource)) {
	t.Helper()
	context := subsystems.BasicClientContext{DataSourceUpdateSink: p.updates}
	ds, err := p.td.Build(context)
	require.NoError(t, err)
	defer ds.Close()

	closer := make(chan struct{})
	ds.Start(closer)
	if !th.AssertChannelClosed(t, closer, time.Millisecond, "start did not close channel") {
		t.FailNow()
	}
	p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)

	action(ds)
}

func TestTestDataSource(t *testing.T) {
	t.Run("initializes with empty data", func(t *testing.T) {
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.withDataSource(t, func(ds subsystems.DataSource) {
				expectedData := ldservices.NewServerSDKData()
				p.updates.DataStore.WaitForInit(t, expectedData, time.Millisecond)
				assert.True(t, ds.IsInitialized())
			})
		})
	})

	t.Run("initializes with flags", func(t *testing.T) {
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.td.Update(p.td.Flag("flag1").On(true)).
				Update(p.td.Flag("flag2").On(false))

			p.withDataSource(t, func(subsystems.DataSource) {
				initData := p.updates.DataStore.WaitForNextInit(t, time.Millisecond)
				dataMap := sharedtest.DataSetToMap(initData)
				require.Len(t, dataMap, 2)
				flags := dataMap[ldstoreimpl.Features()]
				require.Len(t, flags, 2)

				assert.Equal(t, 1, flags["flag1"].Version)
				assert.Equal(t, 1, flags["flag2"].Version)
				assert.True(t, flags["flag1"].Item.(*ldmodel.FeatureFlag).On)
				assert.False(t, flags["flag2"].Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})

	t.Run("adds flag", func(t *testing.T) {
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.withDataSource(t, func(subsystems.DataSource) {
				p.td.Update(p.td.Flag("flag1").On(true))

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flag1", 1, time.Millisecond)
				assert.True(t, up.Item.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})

	t.Run("updates flag", func(t *testing.T) {
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.td.Update(p.td.Flag("flag1").On(false))

			p.withDataSource(t, func(subsystems.DataSource) {
				p.td.Update(p.td.Flag("flag1").On(true))

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flag1", 2, time.Millisecond)
				assert.True(t, up.Item.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})

	t.Run("updates status", func(t *testing.T) {
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.withDataSource(t, func(subsystems.DataSource) {
				ei := interfaces.DataSourceErrorInfo{Kind: interfaces.DataSourceErrorKindNetworkError}
				p.td.UpdateStatus(interfaces.DataSourceStateInterrupted, ei)

				status := p.updates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
				assert.Equal(t, ei, status.LastError)
			})
		})
	})

	t.Run("adds or updates preconfigured flag", func(t *testing.T) {
		flagv1 := ldbuilders.NewFlagBuilder("flagkey").Version(1).On(true).TrackEvents(true).Build()
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.withDataSource(t, func(subsystems.DataSource) {
				p.td.UsePreconfiguredFlag(flagv1)

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), flagv1.Key, 1, time.Millisecond)
				assert.Equal(t, &flagv1, up.Item.Item.(*ldmodel.FeatureFlag))

				updatedFlag := flagv1
				updatedFlag.On = false
				expectedFlagV2 := updatedFlag
				expectedFlagV2.Version = 2
				p.td.UsePreconfiguredFlag(updatedFlag)

				up = p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), flagv1.Key, 2, time.Millisecond)
				assert.Equal(t, &expectedFlagV2, up.Item.Item.(*ldmodel.FeatureFlag))
			})
		})
	})

	t.Run("adds or updates preconfigured segment", func(t *testing.T) {
		segmentv1 := ldbuilders.NewSegmentBuilder("segmentkey").Version(1).Included("a").Build()
		testDataSourceTest(t, func(p testDataSourceTestParams) {
			p.withDataSource(t, func(subsystems.DataSource) {
				p.td.UsePreconfiguredSegment(segmentv1)

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Segments(), segmentv1.Key, 1, time.Millisecond)
				assert.Equal(t, &segmentv1, up.Item.Item.(*ldmodel.Segment))

				updatedSegment := segmentv1
				updatedSegment.Included = []string{"b"}
				expectedSegmentV2 := updatedSegment
				expectedSegmentV2.Version = 2
				p.td.UsePreconfiguredSegment(updatedSegment)

				up = p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Segments(), segmentv1.Key, 2, time.Millisecond)
				assert.Equal(t, &expectedSegmentV2, up.Item.Item.(*ldmodel.Segment))
			})
		})
	})
}
