package ldtestdata

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"

	"github.com/launchdarkly/go-test-helpers/v2/ldservices"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	threeStringValues = []ldvalue.Value{ldvalue.String("red"), ldvalue.String("green"), ldvalue.String("blue")}
)

type testDataSourceTestParams struct {
	td      *TestDataSource
	updates *sharedtest.MockDataSourceUpdates
}

func testDataSourceTest(action func(testDataSourceTestParams)) {
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	var p testDataSourceTestParams
	p.td = DataSource()
	p.updates = sharedtest.NewMockDataSourceUpdates(store)
	action(p)
}

func (p testDataSourceTestParams) withDataSource(t *testing.T, action func(interfaces.DataSource)) {
	ds, err := p.td.CreateDataSource(nil, p.updates)
	require.NoError(t, err)
	defer ds.Close()

	closer := make(chan struct{})
	ds.Start(closer)
	select {
	case _, ok := <-closer:
		require.False(t, ok)
	default:
		require.Fail(t, "start did not close channel")
	}
	p.updates.RequireStatusOf(t, interfaces.DataSourceStateValid)

	action(ds)
}

func TestTestDataSource(t *testing.T) {
	t.Run("initializes with empty data", func(t *testing.T) {
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.withDataSource(t, func(ds interfaces.DataSource) {
				expectedData := ldservices.NewServerSDKData()
				p.updates.DataStore.WaitForInit(t, expectedData, time.Millisecond)
				assert.True(t, ds.IsInitialized())
			})
		})
	})

	t.Run("initializes with flags", func(t *testing.T) {
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.td.Update(p.td.Flag("flag1").On(true)).
				Update(p.td.Flag("flag2").On(false))

			p.withDataSource(t, func(interfaces.DataSource) {
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
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.withDataSource(t, func(interfaces.DataSource) {
				p.td.Update(p.td.Flag("flag1").On(true))

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flag1", 1, time.Millisecond)
				assert.True(t, up.Item.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})

	t.Run("updates flag", func(t *testing.T) {
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.td.Update(p.td.Flag("flag1").On(false))

			p.withDataSource(t, func(interfaces.DataSource) {
				p.td.Update(p.td.Flag("flag1").On(true))

				up := p.updates.DataStore.WaitForUpsert(t, ldstoreimpl.Features(), "flag1", 2, time.Millisecond)
				assert.True(t, up.Item.Item.(*ldmodel.FeatureFlag).On)
			})
		})
	})

	t.Run("updates status", func(t *testing.T) {
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.withDataSource(t, func(interfaces.DataSource) {
				ei := interfaces.DataSourceErrorInfo{Kind: interfaces.DataSourceErrorKindNetworkError}
				p.td.UpdateStatus(interfaces.DataSourceStateInterrupted, ei)

				status := p.updates.RequireStatusOf(t, interfaces.DataSourceStateInterrupted)
				assert.Equal(t, ei, status.LastError)
			})
		})
	})

	t.Run("adds or updates preconfigured flag", func(t *testing.T) {
		flagv1 := ldbuilders.NewFlagBuilder("flagkey").Version(1).On(true).TrackEvents(true).Build()
		testDataSourceTest(func(p testDataSourceTestParams) {
			p.withDataSource(t, func(interfaces.DataSource) {
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
}
