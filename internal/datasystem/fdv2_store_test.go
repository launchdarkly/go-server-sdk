package datasystem

import (
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datastatus"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/stretchr/testify/assert"
)

func TestStore_New(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	assert.NoError(t, store.Close())
}

func TestStore_NoPersistence_NewStore_DataStatus(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.Equal(t, store.DataAvailability(), Defaults)
}

func TestStore_NoPersistence_NewStore_IsInitialized(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.False(t, store.IsInitialized())
}

func TestStore_NoPersistence_MemoryStoreInitialized_DataStatus(t *testing.T) {
	tests := []struct {
		name       string
		datastatus datastatus.DataStatus
		expected   DataAvailability
	}{
		{"fresh data", datastatus.Authoritative, Refreshed},
		{"stale data", datastatus.Derivative, Cached},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logCapture := ldlogtest.NewMockLog()
			store := NewStore(logCapture.Loggers)
			defer store.Close()
			store.Init([]ldstoretypes.Collection{}, tt.datastatus)
			assert.Equal(t, store.DataAvailability(), tt.expected)
			assert.True(t, store.IsInitialized())
		})
	}
}

func TestStore_Commit(t *testing.T) {
	t.Run("no persistent store doesn't cause an error", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()
		assert.NoError(t, store.Commit())
	})

	t.Run("refreshed memory items are copied to persistent store in r/w mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		spy := &fakeStore{isDown: true}

		store := NewStore(logCapture.Loggers).WithPersistence(spy, subsystems.StoreModeReadWrite, nil)
		defer store.Close()

		initPayload := []ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
			{Kind: ldstoreimpl.Segments(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "bar", Item: ldstoretypes.ItemDescriptor{Version: 2}},
			}},
		}

		assert.True(t, store.Init(initPayload, datastatus.Authoritative))

		require.Empty(t, spy.initPayload)

		spy.isDown = false

		require.NoError(t, store.Commit())

		assert.Equal(t, initPayload, spy.initPayload)
	})

	t.Run("stale memory items are not copied to persistent store in r/w mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		spy := &fakeStore{}
		store := NewStore(logCapture.Loggers).WithPersistence(&fakeStore{}, subsystems.StoreModeReadWrite, nil)
		defer store.Close()

		initPayload := []ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
			{Kind: ldstoreimpl.Segments(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "bar", Item: ldstoretypes.ItemDescriptor{Version: 2}},
			}},
		}

		assert.True(t, store.Init(initPayload, datastatus.Derivative))

		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		assert.Empty(t, spy.initPayload)
	})

	t.Run("refreshed memory items are not copied to persistent store in r-only mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		spy := &fakeStore{}
		store := NewStore(logCapture.Loggers).WithPersistence(spy, subsystems.StoreModeRead, nil)
		defer store.Close()

		initPayload := []ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
			{Kind: ldstoreimpl.Segments(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "bar", Item: ldstoretypes.ItemDescriptor{Version: 2}},
			}},
		}

		assert.True(t, store.Init(initPayload, datastatus.Authoritative))

		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		assert.Empty(t, spy.initPayload)
	})
}

func TestStore_GetActive(t *testing.T) {
	t.Run("memory store is active if no persistent store configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()
		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, foo, ldstoretypes.ItemDescriptor{}.NotFound())

		assert.True(t, store.Init([]ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
		}, datastatus.Authoritative))

		foo, err = store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})

	t.Run("persistent store is active if configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers).WithPersistence(&fakeStore{}, subsystems.StoreModeReadWrite, nil)
		defer store.Close()

		_, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.Equal(t, errImAPersistentStore, err)
	})

	t.Run("active store swaps from persistent to memory", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers).WithPersistence(&fakeStore{}, subsystems.StoreModeReadWrite, nil)
		defer store.Close()

		_, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.Equal(t, errImAPersistentStore, err)

		assert.True(t, store.Init([]ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
		}, datastatus.Authoritative))

		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})
}

func TestStore_Concurrency(t *testing.T) {
	t.Run("methods using the active store", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()

		var wg sync.WaitGroup
		go func() {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = store.DataAvailability()
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}()
		go func() {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = store.Get(ldstoreimpl.Features(), "foo")
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}()

		go func() {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = store.GetAll(ldstoreimpl.Features())
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}()
		go func() {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = store.IsInitialized()
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}()
		go func() {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = store.Init([]ldstoretypes.Collection{}, datastatus.Authoritative)
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}()
	})
}

type fakeStore struct {
	initPayload []ldstoretypes.Collection
	isDown      bool
}

var errImAPersistentStore = errors.New("i'm a persistent store")

func (f *fakeStore) GetAll(kind ldstoretypes.DataKind) ([]ldstoretypes.KeyedItemDescriptor, error) {
	return nil, nil
}

func (f *fakeStore) Get(kind ldstoretypes.DataKind, key string) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{}, errImAPersistentStore
}

func (f *fakeStore) IsInitialized() bool {
	return false
}

func (f *fakeStore) Init(allData []ldstoretypes.Collection) error {
	if f.isDown {
		return errors.New("store is down")
	}
	f.initPayload = allData
	return nil
}

func (f *fakeStore) Upsert(kind ldstoretypes.DataKind, key string, item ldstoretypes.ItemDescriptor) (bool, error) {
	return false, nil
}

func (f *fakeStore) IsStatusMonitoringEnabled() bool {
	return false
}

func (f *fakeStore) Close() error {
	return nil
}
