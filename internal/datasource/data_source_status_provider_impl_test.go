package datasource

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

type dataSourceStatusProviderImplTestParams struct {
	dataSourceStatusProvider interfaces.DataSourceStatusProvider
	dataSourceUpdates        *DataSourceUpdatesImpl
}

func dataSourceStatusProviderImplTest(action func(dataSourceStatusProviderImplTestParams)) {
	p := dataSourceStatusProviderImplTestParams{}
	statusBroadcaster := internal.NewDataSourceStatusBroadcaster()
	defer statusBroadcaster.Close()
	flagBroadcaster := internal.NewFlagChangeEventBroadcaster()
	defer flagBroadcaster.Close()
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	dataStoreStatusProvider := datastore.NewDataStoreStatusProviderImpl(store, nil)
	p.dataSourceUpdates = NewDataSourceUpdatesImpl(store, dataStoreStatusProvider, statusBroadcaster, flagBroadcaster,
		0, sharedtest.NewTestLoggers())
	p.dataSourceStatusProvider = NewDataSourceStatusProviderImpl(statusBroadcaster, p.dataSourceUpdates)

	action(p)
}

func makeDataSourceErrorInfo() intf.DataSourceErrorInfo {
	return intf.DataSourceErrorInfo{Kind: intf.DataSourceErrorKindUnknown, Message: "sorry", Time: time.Now()}
}

func TestDataSourceStatusProviderImpl(t *testing.T) {
	t.Run("GetStatus", func(t *testing.T) {
		dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
			errorInfo := makeDataSourceErrorInfo()
			p.dataSourceUpdates.UpdateStatus(intf.DataSourceStateOff, errorInfo)

			status := p.dataSourceStatusProvider.GetStatus()
			assert.Equal(t, intf.DataSourceStateOff, status.State)
			assert.Equal(t, errorInfo, status.LastError)
		})
	})

	t.Run("listeners", func(t *testing.T) {
		dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
			ch1 := p.dataSourceStatusProvider.AddStatusListener()
			ch2 := p.dataSourceStatusProvider.AddStatusListener()
			ch3 := p.dataSourceStatusProvider.AddStatusListener()
			p.dataSourceStatusProvider.RemoveStatusListener(ch2)

			errorInfo := makeDataSourceErrorInfo()
			p.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)

			require.Len(t, ch1, 1)
			require.Len(t, ch2, 0)
			require.Len(t, ch3, 1)
			status1 := <-ch1
			status3 := <-ch3
			assert.Equal(t, intf.DataSourceStateOff, status1.State)
			assert.Equal(t, errorInfo, status1.LastError)
			assert.Equal(t, status1, status3)
		})
	})

	t.Run("WaitFor", func(t *testing.T) {
		t.Run("returns true immediately when status is already correct", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				p.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})

				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)
				assert.True(t, success)
			})
		})

		t.Run("returns false immediately when status is already Off", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				timeStart := time.Now()
				p.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{})
				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)
				assert.False(t, success)
				assert.True(t, time.Now().Sub(timeStart) < 500*time.Millisecond)
			})
		})

		t.Run("succeeds after status change", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				go func() {
					<-time.After(100 * time.Millisecond)
					p.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
				}()
				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)
				assert.True(t, success)
			})
		})

		t.Run("times out", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				timeStart := time.Now()
				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 300*time.Millisecond)
				assert.False(t, success)
				assert.True(t, time.Now().Sub(timeStart) >= 270*time.Millisecond)
			})
		})

		t.Run("ends if shut down", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				go func() {
					<-time.After(100 * time.Millisecond)
					p.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateOff, interfaces.DataSourceErrorInfo{})
				}()
				timeStart := time.Now()
				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)
				assert.False(t, success)
				assert.True(t, time.Now().Sub(timeStart) < 500*time.Millisecond)
			})
		})
	})
}
