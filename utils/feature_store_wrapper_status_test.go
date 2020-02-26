package utils

import (
	"errors"
	"sync"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldbuilders"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

// Test implementation of DataStoreCore with DataStoreCoreStatus.
type mockCoreWithStatus struct {
	cacheTTL         time.Duration
	data             map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData
	fakeError        error
	fakeAvailability bool
	inited           bool
	initQueriedCount int
	lock             sync.Mutex
}

func newCoreWithStatus(ttl time.Duration) *mockCoreWithStatus {
	return &mockCoreWithStatus{
		cacheTTL:         ttl,
		data:             map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData{interfaces.DataKindFeatures(): {}, interfaces.DataKindSegments(): {}},
		fakeAvailability: true,
	}
}

func (c *mockCoreWithStatus) forceSet(kind interfaces.VersionedDataKind, item interfaces.VersionedData) {
	c.data[kind][item.GetKey()] = item
}

func (c *mockCoreWithStatus) forceRemove(kind interfaces.VersionedDataKind, key string) {
	delete(c.data[kind], key)
}

func (c *mockCoreWithStatus) GetCacheTTL() time.Duration {
	return c.cacheTTL
}

func (c *mockCoreWithStatus) InitInternal(allData map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	if c.fakeError != nil {
		return c.fakeError
	}
	c.data = allData
	c.inited = true
	return nil
}

func (c *mockCoreWithStatus) GetInternal(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind][key], nil
}

func (c *mockCoreWithStatus) GetAllInternal(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	return c.data[kind], nil
}

func (c *mockCoreWithStatus) UpsertInternal(kind interfaces.VersionedDataKind, item interfaces.VersionedData) (interfaces.VersionedData, error) {
	if c.fakeError != nil {
		return nil, c.fakeError
	}
	oldItem := c.data[kind][item.GetKey()]
	if oldItem != nil && oldItem.GetVersion() >= item.GetVersion() {
		return oldItem, nil
	}
	c.data[kind][item.GetKey()] = item
	return item, nil
}

func (c *mockCoreWithStatus) InitializedInternal() bool {
	c.initQueriedCount++
	return c.inited
}

func (c *mockCoreWithStatus) IsStoreAvailable() bool {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.fakeAvailability
}

func (c *mockCoreWithStatus) setAvailable(available bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.fakeAvailability = available
}

func consumeStatusWithTimeout(t *testing.T, subCh <-chan internal.DataStoreStatus, timeout time.Duration) internal.DataStoreStatus {
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

	runTests := func(t *testing.T, name string, test func(t *testing.T, mode testCacheMode, core *mockCoreWithStatus),
		forModes ...testCacheMode) {
		t.Run(name, func(t *testing.T) {
			for _, mode := range forModes {
				t.Run(string(mode), func(t *testing.T) {
					test(t, mode, newCoreWithStatus(mode.ttl()))
				})
			}
		})
	}

	runTests(t, "Status is OK initially", func(t *testing.T, mode testCacheMode, core *mockCoreWithStatus) {
		w := NewDataStoreWrapperWithConfig(core, ldlog.NewDisabledLoggers())
		defer w.Close()
		assert.Equal(t, internal.DataStoreStatus{Available: true}, w.GetStoreStatus())
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Status is unavailable after error", func(t *testing.T, mode testCacheMode, core *mockCoreWithStatus) {
		w := NewDataStoreWrapperWithConfig(core, ldlog.NewDisabledLoggers())
		defer w.Close()

		core.fakeError = errors.New("sorry")
		_, err := w.All(interfaces.DataKindFeatures())
		require.Equal(t, core.fakeError, err)

		assert.Equal(t, internal.DataStoreStatus{Available: false}, w.GetStoreStatus())
	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Error listener is notified on failure and recovery", func(t *testing.T, mode testCacheMode, core *mockCoreWithStatus) {
		w := NewDataStoreWrapperWithConfig(core, ldlog.NewDisabledLoggers())
		defer w.Close()
		sub := w.StatusSubscribe()
		require.NotNil(t, sub)
		defer sub.Close()

		core.fakeError = errors.New("sorry")
		core.setAvailable(false)
		_, err := w.All(interfaces.DataKindFeatures())
		require.Equal(t, core.fakeError, err)

		updatedStatus := consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		require.Equal(t, internal.DataStoreStatus{Available: false}, updatedStatus)

		// Trigger another error, just to show that it will *not* publish a redundant status update since it
		// is already in a failed state - the consumeStatusWithTimeout call below will get the success update
		_, err = w.All(interfaces.DataKindFeatures())
		require.Equal(t, core.fakeError, err)

		// Now simulate the data store becoming OK again; the poller detects this and publishes a new status
		core.setAvailable(true)
		updatedStatus = consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		expectedStatus := internal.DataStoreStatus{
			Available:    true,
			NeedsRefresh: mode != testCachedIndefinitely,
		}
		assert.Equal(t, expectedStatus, updatedStatus)
	}, testUncached, testCached, testCachedIndefinitely)

	t.Run("Cache is written to store after recovery if TTL is infinite", func(t *testing.T) {
		core := newCoreWithStatus(-1)
		w := NewDataStoreWrapperWithConfig(core, ldlog.NewDisabledLoggers())
		defer w.Close()
		sub := w.StatusSubscribe()
		require.NotNil(t, sub)
		defer sub.Close()

		core.fakeError = errors.New("sorry")
		core.setAvailable(false)
		_, err := w.All(interfaces.DataKindFeatures())
		require.Equal(t, core.fakeError, err)

		updatedStatus := consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		require.Equal(t, internal.DataStoreStatus{Available: false}, updatedStatus)

		// While the store is still down, try to update it - the update goes into the cache
		flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
		err = w.Upsert(interfaces.DataKindFeatures(), &flag)
		assert.Equal(t, core.fakeError, err)
		cachedFlag, err := w.Get(interfaces.DataKindFeatures(), flag.Key)
		assert.NoError(t, err)
		assert.Equal(t, &flag, cachedFlag)

		// Verify that this update did not go into the underlying data yet
		assert.Nil(t, core.data[interfaces.DataKindFeatures()][flag.Key])

		// Now simulate the store coming back up
		core.fakeError = nil
		core.setAvailable(true)

		// Wait for the poller to notice this and publish a new status
		updatedStatus = consumeStatusWithTimeout(t, sub.Channel(), statusUpdateTimeout)
		assert.Equal(t, internal.DataStoreStatus{Available: true}, updatedStatus)

		// Once that has happened, the cache should have been written to the store
		assert.Equal(t, &flag, core.data[interfaces.DataKindFeatures()][flag.Key])
	})
}
