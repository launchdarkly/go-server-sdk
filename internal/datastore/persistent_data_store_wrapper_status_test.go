package datastore

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	intf "github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoretypes"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dataStoreStatusTestParams struct {
	store            subsystems.DataStore
	core             *mocks.MockPersistentDataStore
	dataStoreUpdates *DataStoreUpdateSinkImpl
	broadcaster      *internal.Broadcaster[interfaces.DataStoreStatus]
}

func withDataStoreStatusTestParams(mode testCacheMode, action func(dataStoreStatusTestParams)) {
	params := dataStoreStatusTestParams{}
	params.broadcaster = internal.NewBroadcaster[interfaces.DataStoreStatus]()
	defer params.broadcaster.Close()
	params.dataStoreUpdates = NewDataStoreUpdateSinkImpl(params.broadcaster)
	params.core = mocks.NewMockPersistentDataStore()
	params.store = NewPersistentDataStoreWrapper(params.core, params.dataStoreUpdates, mode.ttl(), sharedtest.NewTestLoggers())
	defer params.store.Close()
	action(params)
}

func TestDataStoreWrapperStatus(t *testing.T) {
	statusUpdateTimeout := 1 * time.Second // status poller has an interval of 500ms

	runTests := func(t *testing.T, name string, test func(t *testing.T, mode testCacheMode),
		forModes ...testCacheMode) {
		t.Run(name, func(t *testing.T) {
			for _, mode := range forModes {
				t.Run(string(mode), func(t *testing.T) { test(t, mode) })
			}
		})
	}

	runTests(t, "Status is unavailable after error (Get)", func(t *testing.T, mode testCacheMode) {
		withDataStoreStatusTestParams(mode, func(p dataStoreStatusTestParams) {
			myError := errors.New("sorry")
			p.core.SetFakeError(myError)
			_, err := p.store.Get(datakinds.Features, "key")
			require.Equal(t, myError, err)

			status := p.dataStoreUpdates.getStatus()
			assert.Equal(t, intf.DataStoreStatus{Available: false}, status)
		})

	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Status is unavailable after error (GetAll)", func(t *testing.T, mode testCacheMode) {
		withDataStoreStatusTestParams(mode, func(p dataStoreStatusTestParams) {
			myError := errors.New("sorry")
			p.core.SetFakeError(myError)
			_, err := p.store.GetAll(datakinds.Features)
			require.Equal(t, myError, err)

			status := p.dataStoreUpdates.getStatus()
			assert.Equal(t, intf.DataStoreStatus{Available: false}, status)
		})

	}, testUncached, testCached, testCachedIndefinitely)

	runTests(t, "Error listener is notified on failure and recovery", func(t *testing.T, mode testCacheMode) {
		withDataStoreStatusTestParams(mode, func(p dataStoreStatusTestParams) {
			statusCh := p.broadcaster.AddListener()

			myError := errors.New("sorry")
			p.core.SetFakeError(myError)
			p.core.SetAvailable(false)
			_, err := p.store.GetAll(datakinds.Features)
			require.Equal(t, myError, err)

			updatedStatus := th.RequireValue(t, statusCh, statusUpdateTimeout)
			require.Equal(t, intf.DataStoreStatus{Available: false}, updatedStatus)

			// Trigger another error, just to show that it will *not* publish a redundant status update since it
			// is already in a failed state - the consumeStatusWithTimeout call below will get the success update
			_, err = p.store.GetAll(datakinds.Features)
			require.Equal(t, myError, err)
			assert.Len(t, statusCh, 0)

			// Wait for at least one status poll interval
			<-time.After(statusPollInterval + time.Millisecond*100)

			// Now simulate the data store becoming OK again; the poller detects this and publishes a new status
			p.core.SetAvailable(true)
			updatedStatus = th.RequireValue(t, statusCh, statusUpdateTimeout)
			expectedStatus := intf.DataStoreStatus{
				Available:    true,
				NeedsRefresh: mode != testCachedIndefinitely,
			}
			assert.Equal(t, expectedStatus, updatedStatus)
		})
	}, testUncached, testCached, testCachedIndefinitely)

	t.Run("Cache is written to store after recovery if TTL is infinite", func(t *testing.T) {
		withDataStoreStatusTestParams(testCachedIndefinitely, func(p dataStoreStatusTestParams) {
			statusCh := p.broadcaster.AddListener()

			myError := errors.New("sorry")
			p.core.SetFakeError(myError)
			p.core.SetAvailable(false)
			_, err := p.store.GetAll(datakinds.Features)
			require.Equal(t, myError, err)

			updatedStatus := th.RequireValue(t, statusCh, statusUpdateTimeout)
			require.Equal(t, intf.DataStoreStatus{Available: false}, updatedStatus)

			// While the store is still down, try to update it - the update goes into the cache
			flag := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
			_, err = p.store.Upsert(datakinds.Features, flag.Key, sharedtest.FlagDescriptor(flag))
			assert.Equal(t, myError, err)
			cachedFlag, err := p.store.Get(datakinds.Features, flag.Key)
			assert.NoError(t, err)
			assert.Equal(t, &flag, cachedFlag.Item)

			// Verify that this update did not go into the underlying data yet
			assert.Equal(t, ldstoretypes.SerializedItemDescriptor{}.NotFound(), p.core.ForceGet(datakinds.Features, flag.Key))

			// Now simulate the store coming back up
			p.core.SetFakeError(nil)
			p.core.SetAvailable(true)

			// Wait for the poller to notice this and publish a new status
			updatedStatus = th.RequireValue(t, statusCh, statusUpdateTimeout)
			assert.Equal(t, intf.DataStoreStatus{Available: true}, updatedStatus)

			// Once that has happened, the cache should have been written to the store
			assert.Equal(t, flag.Version, p.core.ForceGet(datakinds.Features, flag.Key).Version)
		})
	})
}
