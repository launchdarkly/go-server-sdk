package datasource

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	intf "github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
)

type dataSourceStatusProviderImplTestParams struct {
	dataSourceStatusProvider interfaces.DataSourceStatusProvider
	dataSourceUpdates        *DataSourceUpdateSinkImpl
}

func dataSourceStatusProviderImplTest(action func(dataSourceStatusProviderImplTestParams)) {
	p := dataSourceStatusProviderImplTestParams{}
	statusBroadcaster := internal.NewBroadcaster[interfaces.DataSourceStatus]()
	defer statusBroadcaster.Close()
	flagBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	defer flagBroadcaster.Close()
	store := datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers())
	dataStoreStatusProvider := datastore.NewDataStoreStatusProviderImpl(store, nil)
	p.dataSourceUpdates = NewDataSourceUpdateSinkImpl(
		store,
		dataStoreStatusProvider,
		statusBroadcaster,
		flagBroadcaster,
		0,
		sharedtest.NewTestLoggers(),
	)
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

		t.Run("returns after timeout while status updates are queued", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				// Flood the status broadcaster so that, when the wait times out, broadcasts
				// wait on the full listener channel. The removal of the listener inside
				// WaitFor must still complete.
				stop := make(chan struct{})
				var flooders sync.WaitGroup
				for i := 0; i < 8; i++ {
					flooders.Add(1)
					go func() {
						defer flooders.Done()
						status := intf.DataSourceStatus{State: intf.DataSourceStateInterrupted}
						for {
							select {
							case <-stop:
								return
							default:
								p.dataSourceUpdates.dataSourceStatusBroadcaster.Broadcast(status)
							}
						}
					}()
				}

				returned := make(chan bool, 1)
				go func() {
					returned <- p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 50*time.Millisecond)
				}()
				select {
				case success := <-returned:
					assert.False(t, success)
				case <-time.After(2 * time.Second):
					t.Error("WaitFor did not return after its timeout")
				}
				close(stop)
				flooders.Wait()
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

		t.Run("ends if status broadcaster is closed", func(t *testing.T) {
			dataSourceStatusProviderImplTest(func(p dataSourceStatusProviderImplTestParams) {
				go func() {
					<-time.After(100 * time.Millisecond)
					p.dataSourceUpdates.dataSourceStatusBroadcaster.Close()
				}()
				timeStart := time.Now()
				success := p.dataSourceStatusProvider.WaitFor(interfaces.DataSourceStateValid, 500*time.Millisecond)
				assert.False(t, success)
				assert.True(t, time.Now().Sub(timeStart) < 500*time.Millisecond)
			})
		})
	})
}
