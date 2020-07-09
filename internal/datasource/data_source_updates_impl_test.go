package datasource

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	st "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

const testDataSourceOutageTimeout = 200 * time.Millisecond

type dataSourceUpdatesImplTestParams struct {
	store                   *sharedtest.CapturingDataStore
	dataStoreStatusProvider intf.DataStoreStatusProvider
	dataSourceUpdates       *DataSourceUpdatesImpl
	flagChangeBroadcaster   *internal.FlagChangeEventBroadcaster
	mockLoggers             *ldlogtest.MockLog
}

func dataSourceUpdatesImplTest(action func(dataSourceUpdatesImplTestParams)) {
	p := dataSourceUpdatesImplTestParams{}
	p.mockLoggers = ldlogtest.NewMockLog()
	p.store = sharedtest.NewCapturingDataStore(datastore.NewInMemoryDataStore(p.mockLoggers.Loggers))
	dataStoreUpdates := datastore.NewDataStoreUpdatesImpl(nil)
	p.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(p.store, dataStoreUpdates)
	dataSourceStatusBroadcaster := internal.NewDataSourceStatusBroadcaster()
	defer dataSourceStatusBroadcaster.Close()
	p.flagChangeBroadcaster = internal.NewFlagChangeEventBroadcaster()
	defer p.flagChangeBroadcaster.Close()
	p.dataSourceUpdates = NewDataSourceUpdatesImpl(
		p.store,
		p.dataStoreStatusProvider,
		dataSourceStatusBroadcaster,
		p.flagChangeBroadcaster,
		testDataSourceOutageTimeout,
		p.mockLoggers.Loggers,
	)

	action(p)
}

func TestDataSourceUpdatesImpl(t *testing.T) {
	storeError := errors.New("sorry")
	expectedStoreErrorMessage := "Unexpected data store error when trying to store an update received from the data source: sorry"

	t.Run("Init", func(t *testing.T) {
		t.Run("passes data to store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				inputData := sharedtest.NewDataSetBuilder().Flags(ldbuilders.NewFlagBuilder("a").Build())

				result := p.dataSourceUpdates.Init(inputData.Build())
				assert.True(t, result)

				p.store.WaitForInit(t, inputData.ToServerSDKData(), time.Second)
			})
		})

		t.Run("detects error from store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.store.SetFakeError(storeError)

				result := p.dataSourceUpdates.Init(sharedtest.NewDataSetBuilder().Build())
				assert.False(t, result)
				assert.Equal(t, intf.DataSourceErrorKindStoreError, p.dataSourceUpdates.GetLastStatus().LastError.Kind)

				log1 := p.mockLoggers.GetOutput(ldlog.Warn)
				assert.Equal(t, []string{expectedStoreErrorMessage}, log1)

				// does not log a redundant message if the next update also fails
				assert.False(t, p.dataSourceUpdates.Init(sharedtest.NewDataSetBuilder().Build()))
				log2 := p.mockLoggers.GetOutput(ldlog.Warn)
				assert.Equal(t, log1, log2)

				// does log the message again if there's another failure later after a success
				p.store.SetFakeError(nil)
				assert.True(t, p.dataSourceUpdates.Init(sharedtest.NewDataSetBuilder().Build()))
				p.store.SetFakeError(storeError)
				assert.False(t, p.dataSourceUpdates.Init(sharedtest.NewDataSetBuilder().Build()))
				log3 := p.mockLoggers.GetOutput(ldlog.Warn)
				assert.Equal(t, []string{expectedStoreErrorMessage, expectedStoreErrorMessage}, log3)
			})
		})

		t.Run("sorts the data set", testDataSourceUpdatesImplSortsInitData)
	})

	t.Run("Upsert", func(t *testing.T) {
		t.Run("passes data to store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				flag := ldbuilders.NewFlagBuilder("key").Version(1).Build()
				itemDesc := st.ItemDescriptor{Version: 1, Item: &flag}
				result := p.dataSourceUpdates.Upsert(datakinds.Features, flag.Key, itemDesc)
				assert.True(t, result)

				p.store.WaitForUpsert(t, datakinds.Features, flag.Key, itemDesc.Version, time.Second)
			})
		})

		t.Run("detects error from store", func(t *testing.T) {
			dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
				p.store.SetFakeError(storeError)

				flag := ldbuilders.NewFlagBuilder("key").Version(1).Build()
				itemDesc := st.ItemDescriptor{Version: 1, Item: &flag}
				result := p.dataSourceUpdates.Upsert(datakinds.Features, flag.Key, itemDesc)
				assert.False(t, result)
				assert.Equal(t, intf.DataSourceErrorKindStoreError, p.dataSourceUpdates.GetLastStatus().LastError.Kind)

				log1 := p.mockLoggers.GetOutput(ldlog.Warn)
				assert.Equal(t, []string{expectedStoreErrorMessage}, log1)

				// does not log a redundant message if the next update also fails
				assert.False(t, p.dataSourceUpdates.Upsert(datakinds.Features, flag.Key, itemDesc))
				log2 := p.mockLoggers.GetOutput(ldlog.Warn)
				assert.Equal(t, log1, log2)

				// does log the message again if there's another failure later after a success
				p.store.SetFakeError(nil)
				assert.True(t, p.dataSourceUpdates.Upsert(datakinds.Features, flag.Key, itemDesc))
				p.store.SetFakeError(storeError)
				assert.False(t, p.dataSourceUpdates.Upsert(datakinds.Features, flag.Key, itemDesc))
				log3 := p.mockLoggers.GetOutput(ldlog.Warn)
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

		t.Run("can log outage at Error level after timeout", TestDataSourceOutageLoggingTimeout)
	})

	t.Run("GetDataStoreStatusProvider", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			assert.Equal(t, p.dataStoreStatusProvider, p.dataSourceUpdates.GetDataStoreStatusProvider())
		})
	})
}

func testDataSourceUpdatesImplSortsInitData(t *testing.T) {
	// The logic for this is already tested in data_model_dependencies_test, but here we are verifying
	// that DataSourceUpdatesImpl is actually using that logic.
	dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
		inputData := makeDependencyOrderingDataSourceTestData()

		result := p.dataSourceUpdates.Init(inputData)
		require.True(t, result)

		receivedData := p.store.WaitForNextInit(t, time.Second)

		verifySortedData(t, receivedData, inputData)
	})
}

func TestDataSourceUpdatesImplFlagChangeEvents(t *testing.T) {
	t.Run("sends events on init for newly added flags", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			builder := sharedtest.NewDataSetBuilder().
				Flags(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()).
				Segments(ldbuilders.NewSegmentBuilder("segment1").Version(1).Build())

			p.dataSourceUpdates.Init(builder.Build())

			ch := p.flagChangeBroadcaster.AddListener()

			builder.Flags(ldbuilders.NewFlagBuilder("flag2").Version(1).Build()).
				Segments(ldbuilders.NewSegmentBuilder("segment2").Version(1).Build())
			// the new segment triggers no events since nothing is using it

			p.dataSourceUpdates.Init(builder.Build())

			sharedtest.ExpectFlagChangeEvents(t, ch, "flag2")
		})
	})

	t.Run("sends event on update for newly added flag", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			builder := sharedtest.NewDataSetBuilder().
				Flags(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()).
				Segments(ldbuilders.NewSegmentBuilder("segment1").Version(1).Build())

			p.dataSourceUpdates.Init(builder.Build())

			ch := p.flagChangeBroadcaster.AddListener()

			flag2 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()
			p.dataSourceUpdates.Upsert(datakinds.Features, flag2.Key, st.ItemDescriptor{Version: flag2.Version, Item: &flag2})

			sharedtest.ExpectFlagChangeEvents(t, ch, "flag2")
		})
	})

	t.Run("sends events on init for updated flags", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			builder := sharedtest.NewDataSetBuilder().
				Flags(
					ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
					ldbuilders.NewFlagBuilder("flag2").Version(1).Build(),
				).
				Segments(
					ldbuilders.NewSegmentBuilder("segment1").Version(1).Build(),
					ldbuilders.NewSegmentBuilder("segment2").Version(1).Build(),
				)

			p.dataSourceUpdates.Init(builder.Build())

			ch := p.flagChangeBroadcaster.AddListener()

			builder.Flags(
				ldbuilders.NewFlagBuilder("flag2").Version(2).Build(), // modified flag
			).Segments(
				ldbuilders.NewSegmentBuilder("segment2").Version(2).Build(), // modified segment, but no one is using it
			)

			p.dataSourceUpdates.Init(builder.Build())

			sharedtest.ExpectFlagChangeEvents(t, ch, "flag2")
		})
	})

	t.Run("sends event on update for updated flag", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			builder := sharedtest.NewDataSetBuilder().
				Flags(
					ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
					ldbuilders.NewFlagBuilder("flag2").Version(1).Build(),
				).
				Segments(ldbuilders.NewSegmentBuilder("segment1").Version(1).Build())

			p.dataSourceUpdates.Init(builder.Build())

			ch := p.flagChangeBroadcaster.AddListener()

			flag2 := ldbuilders.NewFlagBuilder("flag2").Version(2).Build()
			p.dataSourceUpdates.Upsert(datakinds.Features, flag2.Key, st.ItemDescriptor{Version: flag2.Version, Item: &flag2})

			sharedtest.ExpectFlagChangeEvents(t, ch, "flag2")
		})
	})

	t.Run("does not send event on update if item was not really updated", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			builder := sharedtest.NewDataSetBuilder().
				Flags(
					ldbuilders.NewFlagBuilder("flag1").Version(1).Build(),
					ldbuilders.NewFlagBuilder("flag2").Version(1).Build(),
				)

			p.dataSourceUpdates.Init(builder.Build())

			ch := p.flagChangeBroadcaster.AddListener()

			flag2 := ldbuilders.NewFlagBuilder("flag2").Version(1).Build()
			p.dataSourceUpdates.Upsert(datakinds.Features, flag2.Key, st.ItemDescriptor{Version: flag2.Version, Item: &flag2})

			sharedtest.ExpectNoMoreFlagChangeEvents(t, ch)
		})
	})
}

func TestDataSourceOutageLoggingTimeout(t *testing.T) {
	t.Run("does not log error if data source recovers before timeout", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			errorInfo := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown}
			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, errorInfo)
			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})

			<-time.After(testDataSourceOutageTimeout)

			assert.Len(t, p.mockLoggers.GetOutput(ldlog.Error), 0)
		})
	})

	t.Run("logs error if data source does not recover before timeout", func(t *testing.T) {
		dataSourceUpdatesImplTest(func(p dataSourceUpdatesImplTestParams) {
			// simulate a series of consecutive errors
			errorInfo1 := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown, Time: time.Now()}
			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, errorInfo1)
			errorInfo2 := intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindErrorResponse, StatusCode: 500, Time: time.Now()}
			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateInterrupted, errorInfo2)

			<-time.After(testDataSourceOutageTimeout + (100 * time.Millisecond))

			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateValid, intf.DataSourceErrorInfo{})

			<-time.After(testDataSourceOutageTimeout)

			require.Len(t, p.mockLoggers.GetOutput(ldlog.Error), 1)
			message := p.mockLoggers.GetOutput(ldlog.Error)[0]
			assert.True(t, strings.HasPrefix(
				message,
				fmt.Sprintf(
					"LaunchDarkly data source outage - updates have been unavailable for at least %s with the following errors:",
					testDataSourceOutageTimeout,
				)))
			assert.Contains(t, message, "UNKNOWN (1 time)")
			assert.Contains(t, message, "ERROR_RESPONSE(500) (1 time)")
		})
	})
}
