package internal

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"

	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

type dataSourceUpdatesImplTestParams struct {
	store                   *sharedtest.CapturingDataStore
	dataStoreStatusProvider intf.DataStoreStatusProvider
	dataSourceUpdates       *DataSourceUpdatesImpl
	mockLoggers             *sharedtest.MockLoggers
}

func dataSourceUpdatesImplTest(action func(dataSourceUpdatesImplTestParams)) {
	p := dataSourceUpdatesImplTestParams{}
	p.mockLoggers = sharedtest.NewMockLoggers()
	p.store = sharedtest.NewCapturingDataStore(NewInMemoryDataStore(p.mockLoggers.Loggers))
	dataStoreUpdates := NewDataStoreUpdatesImpl(nil)
	p.dataStoreStatusProvider = NewDataStoreStatusProviderImpl(p.store, dataStoreUpdates)
	dataSourceStatusBroadcaster := NewDataSourceStatusBroadcaster()
	defer dataSourceStatusBroadcaster.Close()
	p.dataSourceUpdates = NewDataSourceUpdatesImpl(p.store, p.dataStoreStatusProvider, dataSourceStatusBroadcaster, p.mockLoggers.Loggers)

	action(p)
}

func TestDataSourceUpdatesImpl(t *testing.T) {
	storeError := errors.New("sorry")
	expectedStoreErrorMessage := "Unexpected data store error when trying to store an update received from the data source: sorry"

	t.Run("Init", func(t *testing.T) {
		t.Run("passes data to store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				inputData := NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("a").Build())

				result := p.dataSourceUpdates.Init(inputData.Build())
				assert.True(t, result)

				p.store.WaitForInit(t, inputData.ToServerSDKData(), time.Second)
			})
		})

		t.Run("detects error from store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.store.SetFakeError(storeError)

				result := p.dataSourceUpdates.Init(NewDataSetBuilder().Build())
				assert.False(t, result)
				assert.Equal(t, intf.DataSourceErrorKindStoreError, p.dataSourceUpdates.GetLastStatus().LastError.Kind)

				log1 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, []string{expectedStoreErrorMessage}, log1)

				// does not log a redundant message if the next update also fails
				assert.False(t, p.dataSourceUpdates.Init(NewDataSetBuilder().Build()))
				log2 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, log1, log2)

				// does log the message again if there's another failure later after a success
				p.store.SetFakeError(nil)
				assert.True(t, p.dataSourceUpdates.Init(NewDataSetBuilder().Build()))
				p.store.SetFakeError(storeError)
				assert.False(t, p.dataSourceUpdates.Init(NewDataSetBuilder().Build()))
				log3 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, []string{expectedStoreErrorMessage, expectedStoreErrorMessage}, log3)
			})
		})

		t.Run("sorts the data set", testDataSourceUpdatesImplSortsInitData)
	})

	t.Run("Upsert", func(t *testing.T) {
		t.Run("passes data to store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				flag := ldbuilders.NewFlagBuilder("key").Version(1).Build()
				itemDesc := intf.StoreItemDescriptor{Version: 1, Item: &flag}
				result := p.dataSourceUpdates.Upsert(intf.DataKindFeatures(), flag.Key, itemDesc)
				assert.True(t, result)

				p.store.WaitForUpsert(t, intf.DataKindFeatures(), flag.Key, itemDesc.Version, time.Second)
			})
		})

		t.Run("detects error from store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.store.SetFakeError(storeError)

				flag := ldbuilders.NewFlagBuilder("key").Version(1).Build()
				itemDesc := intf.StoreItemDescriptor{Version: 1, Item: &flag}
				result := p.dataSourceUpdates.Upsert(intf.DataKindFeatures(), flag.Key, itemDesc)
				assert.False(t, result)
				assert.Equal(t, intf.DataSourceErrorKindStoreError, p.dataSourceUpdates.GetLastStatus().LastError.Kind)

				log1 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, []string{expectedStoreErrorMessage}, log1)

				// does not log a redundant message if the next update also fails
				assert.False(t, p.dataSourceUpdates.Upsert(intf.DataKindFeatures(), flag.Key, itemDesc))
				log2 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, log1, log2)

				// does log the message again if there's another failure later after a success
				p.store.SetFakeError(nil)
				assert.True(t, p.dataSourceUpdates.Upsert(intf.DataKindFeatures(), flag.Key, itemDesc))
				p.store.SetFakeError(storeError)
				assert.False(t, p.dataSourceUpdates.Upsert(intf.DataKindFeatures(), flag.Key, itemDesc))
				log3 := p.mockLoggers.Output[ldlog.Warn]
				assert.Equal(t, []string{expectedStoreErrorMessage, expectedStoreErrorMessage}, log3)
			})
		})
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		// broadcaster behavior is covered by DataSourceStatusProviderImpl tests

		t.Run("does not update status if state is the same and errorInfo is empty", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})
				status1 := p.dataSourceUpdates.currentStatus
				<-time.After(time.Millisecond) // so time is different

				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})
				status2 := p.dataSourceUpdates.currentStatus
				assert.Equal(t, status1, status2)
			})
		})

		t.Run("does not update status if new state is empty", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})
				status1 := p.dataSourceUpdates.currentStatus

				p.dataSourceUpdates.UpdateStatus("", intf.DataSourceErrorInfo{})
				status2 := p.dataSourceUpdates.currentStatus
				assert.Equal(t, status1, status2)
			})
		})

		t.Run("updates status if state is the same and errorInfo is not empty", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})
				status1 := p.dataSourceUpdates.currentStatus
				<-time.After(time.Millisecond) // so time is different

				errorInfo := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown}
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, errorInfo)
				status2 := p.dataSourceUpdates.currentStatus
				assert.NotEqual(t, status1, status2)
				assert.Equal(t, status1.State, status2.State)
				assert.Equal(t, errorInfo, status2.LastError)
			})
		})

		t.Run("updates status if state is not the same", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				errorInfo := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown}
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, errorInfo)

				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, intf.DataSourceErrorInfo{})
				status := p.dataSourceUpdates.currentStatus
				assert.Equal(t, intf.DataSourceStateInterrupted, status.State)
				assert.Equal(t, errorInfo, status.LastError)
			})
		})

		t.Run("Initialized is used instead of Interrupted during startup", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				errorInfo := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown}
				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, errorInfo)
				status1 := p.dataSourceUpdates.currentStatus
				assert.Equal(t, intf.DataSourceStateInitializing, status1.State)
				assert.Equal(t, errorInfo, status1.LastError)

				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})
				status2 := p.dataSourceUpdates.currentStatus
				assert.Equal(t, intf.DataSourceStateValid, status2.State)

				p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, intf.DataSourceErrorInfo{})
				status3 := p.dataSourceUpdates.currentStatus
				assert.Equal(t, intf.DataSourceStateInterrupted, status3.State)
				assert.Equal(t, errorInfo, status3.LastError)
			})
		})
	})

	t.Run("GetDataStoreStatusProvider", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			assert.Equal(t, p.dataStoreStatusProvider, p.dataSourceUpdates.GetDataStoreStatusProvider())
		})
	})
}

func testDataSourceUpdatesImplSortsInitData(t *testing.T) {
	// This verifies that the data store will receive the data set in the correct ordering for flag
	// prerequisites, etc., in case it is not able to do an atomic update.
	dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
		inputData := makeDependencyOrderingDataSourceTestData()

		result := p.dataSourceUpdates.Init(inputData)
		require.True(t, result)

		receivedData := p.store.WaitForNextInit(t, time.Second)

		assert.Equal(t, 2, len(receivedData))

		assert.Equal(t, intf.DataKindSegments(), receivedData[0].Kind) // Segments should always be first
		assert.Equal(t, 1, len(receivedData[0].Items))
		assert.Equal(t, intf.DataKindFeatures(), receivedData[1].Kind)
		assert.Equal(t, 6, len(receivedData[1].Items))

		flags := receivedData[1].Items
		findFlagIndex := func(key string) int {
			for i, item := range flags {
				if item.Key == key {
					return i
				}
			}
			return -1
		}

		for _, item := range inputData[0].Items {
			if flag, ok := item.Item.Item.(*ldmodel.FeatureFlag); ok {
				flagIndex := findFlagIndex(item.Key)
				for _, prereq := range flag.Prerequisites {
					prereqIndex := findFlagIndex(prereq.Key)
					if prereqIndex > flagIndex {
						keys := make([]string, 0, len(flags))
						for _, item := range flags {
							keys = append(keys, item.Key)
						}
						assert.True(t, false, "%s depends on %s, but %s was listed first; keys in order are [%s]",
							flag.Key, prereq.Key, strings.Join(keys, ", "))
					}
				}
			}
		}
	})
}

func makeDependencyOrderingDataSourceTestData() []intf.StoreCollection {
	return NewDataSetBuilder().
		Flags(
			ldbuilders.NewFlagBuilder("a").AddPrerequisite("b", 0).AddPrerequisite("c", 0).Build(),
			ldbuilders.NewFlagBuilder("b").AddPrerequisite("c", 0).AddPrerequisite("e", 0).Build(),
			ldbuilders.NewFlagBuilder("c").Build(),
			ldbuilders.NewFlagBuilder("d").Build(),
			ldbuilders.NewFlagBuilder("e").Build(),
			ldbuilders.NewFlagBuilder("f").Build(),
		).
		Segments(
			ldbuilders.NewSegmentBuilder("1").Build(),
		).
		Build()
}
