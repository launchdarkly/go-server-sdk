package internal

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func consumeStatusWithTimeout(t *testing.T, subCh <-chan DataStoreStatus, timeout time.Duration) DataStoreStatus {
	deadline := time.After(timeout)
	for {
		select {
		case <-deadline:
			require.True(t, false, "did not receive status update after %v", timeout)
		case s := <-subCh:
			return s
		}
	}
}

func TestDataStoreWrapperStatus(t *testing.T) {
	statusUpdateTimeout := 1 * time.Second // status poller has an interval of 500ms

	runTests := func(t *testing.T, name string, test func(t *testing.T, mode testCacheMode, core *sharedtest.MockPersistentDataStore),
		forModes ...testCacheMode) {
		t.Run(name, func(t *testing.T) {
			for _, mode := range forModes {
				t.Run(string(mode), func(t *testing.T) {
					test(t, mode, sharedtest.NewMockPersistentDataStore())
				})
			}
		})
	}

	runTests(t, "Status is OK initially", func(t *testing.T, mode testCacheMode, core *sharedtest.MockPersistentDataStore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()
		assert.Equal(t, DataStoreStatus{Available: true}, w.GetStoreStatus())
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Status is unavailable after error", func(t *testing.T, mode testCacheMode, core *sharedtest.MockPersistentDataStore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()

		myError := errors.New("sorry")
		core.SetFakeError(myError)
		_, err := w.GetAll(intf.DataKindFeatures())
		require.Equal(t, myError, err)

		assert.Equal(t, DataStoreStatus{Available: false}, w.GetStoreStatus())
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Error listener is notified on failure and recovery", func(t *testing.T, mode testCacheMode, core *sharedtest.MockPersistentDataStore) {
		w := NewPersistentDataStoreWrapper(core, mode.ttl(), ldlog.NewDisabledLoggers())
		defer w.Close()
		sub := w.StatusSubscribe()
		require.NotNil(t, sub)
		defer sub.Close()

		myError := errors.New("sorry")
		core.SetFakeError(myError)
		core.SetAvailable(false)
		_, err := w.GetAll(intf.DataKindFeatures())
		require.Equal(t, myError, err)

		updatedStatus := consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		require.Equal(t, DataStoreStatus{Available: false}, updatedStatus)

		// Trigger another error, just to show that it will *not* publish a redundant status update since it
		// is already in a failed state - the consumeStatusWithTimeout call below will get the success update
		_, err = w.GetAll(intf.DataKindFeatures())
		require.Equal(t, myError, err)

		// Now simulate the data store becoming OK again; the poller detects this and publishes a new status
		core.SetAvailable(true)
		updatedStatus = consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		expectedStatus := DataStoreStatus{
			Available:    true,
			NeedsRefresh: mode != testCachedIndefinitely,
		}
		assert.Equal(t, expectedStatus, updatedStatus)
	}, testUncached, testCached, testCachedIndefinitely)

	t.Run("Cache is written to store after recovery if TTL is infinite", func(t *testing.T) {
		core := sharedtest.NewMockPersistentDataStore()
		w := NewPersistentDataStoreWrapper(core, -1, ldlog.NewDisabledLoggers())
		defer w.Close()
		sub := w.StatusSubscribe()
		require.NotNil(t, sub)
		defer sub.Close()

		myError := errors.New("sorry")
		core.SetFakeError(myError)
		core.SetAvailable(false)
		_, err := w.GetAll(intf.DataKindFeatures())
		require.Equal(t, myError, err)

		updatedStatus := consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		require.Equal(t, DataStoreStatus{Available: false}, updatedStatus)

		// While the store is still down, try to update it - the update goes into the cache
		flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		err = w.Upsert(intf.DataKindFeatures(), flag.Key, flagDescriptor(flag))
		assert.Equal(t, myError, err)
		cachedFlag, err := w.Get(intf.DataKindFeatures(), flag.Key)
		assert.NoError(t, err)
		assert.Equal(t, &flag, cachedFlag.Item)

		// Verify that this update did not go into the underlying data yet
		assert.Equal(t, intf.StoreSerializedItemDescriptor{}.NotFound(), core.ForceGet(intf.DataKindFeatures(), flag.Key))

		// Now simulate the store coming back up
		core.SetFakeError(nil)
		core.SetAvailable(true)

		// Wait for the poller to notice this and publish a new status
		updatedStatus = consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		assert.Equal(t, DataStoreStatus{Available: true}, updatedStatus)

		// Once that has happened, the cache should have been written to the store
		assert.Equal(t, flag.Version, core.ForceGet(intf.DataKindFeatures(), flag.Key).Version)
	})
}
