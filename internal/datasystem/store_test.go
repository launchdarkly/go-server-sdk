package datasystem

import (
	"errors"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

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

func TestStore_NoSelector(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.Equal(t, fdv2proto.NoSelector(), store.Selector())
}

func TestStore_NoPersistence_NewStore_IsNotInitialized(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.False(t, store.IsInitialized())
}

func TestStore_NoPersistence_MemoryStore_IsInitialized(t *testing.T) {
	v1 := fdv2proto.NewSelector("foo", 1)
	none := fdv2proto.NoSelector()
	tests := []struct {
		name     string
		selector *fdv2proto.Selector
		persist  bool
	}{
		{"with selector, persist", v1, true},
		{"with selector, do not persist", v1, false},
		{"no selector, persist", none, true},
		{"no selector, do not persist", none, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logCapture := ldlogtest.NewMockLog()
			store := NewStore(logCapture.Loggers)
			defer store.Close()
			assert.NoError(t, store.SetBasis([]fdv2proto.Event{}, tt.selector, tt.persist))
			assert.True(t, store.IsInitialized())
		})
	}
}

func TestStore_Commit(t *testing.T) {
	t.Run("absence of persistent store doesn't cause error when committing", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()
		assert.NoError(t, store.Commit())
	})

	t.Run("persist-marked memory items are copied to persistent store in r/w mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		// isDown causes the fake to reject updates (until flipped to false).
		spy := &fakeStore{isDown: true}

		store := NewStore(logCapture.Loggers).WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		// The store receives data as a list of events, but the persistent store receives them as an
		// []ldstoretypes.Collection.
		input := []fdv2proto.Event{
			fdv2proto.PutObject{Kind: datakinds.Features, Key: "foo", Object: ldstoretypes.ItemDescriptor{Version: 1}},
			fdv2proto.PutObject{Kind: datakinds.Segments, Key: "bar", Object: ldstoretypes.ItemDescriptor{Version: 2}},
		}

		output := []ldstoretypes.Collection{
			{
				Kind: ldstoreimpl.Features(),
				Items: []ldstoretypes.KeyedItemDescriptor{
					{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
				},
			},
			{
				Kind: ldstoreimpl.Segments(),
				Items: []ldstoretypes.KeyedItemDescriptor{
					{Key: "bar", Item: ldstoretypes.ItemDescriptor{Version: 2}},
				},
			}}

		// There should be an error since writing to the store will fail.
		assert.Error(t, store.SetBasis(input, fdv2proto.NoSelector(), true))

		// Since writing should have failed, there should be no data in the persistent store.
		require.Empty(t, spy.initPayload)

		spy.isDown = false

		// This time, the data should be stored properly.
		require.NoError(t, store.Commit())

		requireCollectionsMatch(t, output, spy.initPayload)
	})

	t.Run("non-persist memory items are not copied to persistent store in r/w mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		// The fake should accept updates.
		spy := &fakeStore{isDown: false}
		store := NewStore(logCapture.Loggers).WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		input := []fdv2proto.Event{
			fdv2proto.PutObject{Kind: datakinds.Features, Key: "foo", Object: ldstoretypes.ItemDescriptor{Version: 1}},
			fdv2proto.PutObject{Kind: datakinds.Segments, Key: "bar", Object: ldstoretypes.ItemDescriptor{Version: 2}},
		}

		assert.NoError(t, store.SetBasis(input, fdv2proto.NoSelector(), false))

		// Since SetBasis will immediately mirror the data if persist == true, we can check this is empty now.
		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		// Commit should be a no-op. This tests that the persist status was saved.
		assert.Empty(t, spy.initPayload)
	})

	t.Run("persist-marked memory items are not copied to persistent store in r-only mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		// The fake should accept updates.
		spy := &fakeStore{isDown: false}
		store := NewStore(logCapture.Loggers).WithPersistence(spy, subsystems.DataStoreModeRead, nil)
		defer store.Close()

		input := []fdv2proto.Event{
			fdv2proto.PutObject{Kind: datakinds.Features, Key: "foo", Object: ldstoretypes.ItemDescriptor{Version: 1}},
			fdv2proto.PutObject{Kind: datakinds.Segments, Key: "bar", Object: ldstoretypes.ItemDescriptor{Version: 2}},
		}

		// Even though persist is true, the store was marked as read-only, so it shouldn't be written to.
		assert.NoError(t, store.SetBasis(input, fdv2proto.NoSelector(), true))

		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		// Same with commit.
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

		input := []fdv2proto.Event{
			fdv2proto.PutObject{Kind: datakinds.Features, Key: "foo", Object: ldstoretypes.ItemDescriptor{Version: 1}},
		}

		assert.NoError(t, store.SetBasis(input, fdv2proto.NoSelector(), false))

		foo, err = store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})

	t.Run("persistent store is active if configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		store := NewStore(logCapture.Loggers).WithPersistence(&fakeStore{}, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		_, err := store.Get(ldstoreimpl.Features(), "foo")

		// The fakeStore should return a specific error when Get is called.
		assert.Equal(t, errImAPersistentStore, err)
	})

	t.Run("active store swaps from persistent to memory", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers).WithPersistence(&fakeStore{}, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		// Before there's any data, if we call Get the persistent store should be accessed.
		_, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.Equal(t, errImAPersistentStore, err)

		input := []fdv2proto.Event{
			fdv2proto.PutObject{Kind: datakinds.Features, Key: "foo", Object: ldstoretypes.ItemDescriptor{Version: 1}},
		}

		assert.NoError(t, store.SetBasis(input, fdv2proto.NoSelector(), false))

		// Now that there's memory data, the persistent store should no longer be accessed.
		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})
}

func TestStore_SelectorIsRemembered(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()

	selector1 := fdv2proto.NewSelector("foo", 1)
	selector2 := fdv2proto.NewSelector("bar", 2)
	selector3 := fdv2proto.NewSelector("baz", 3)
	selector4 := fdv2proto.NewSelector("qux", 4)
	selector5 := fdv2proto.NewSelector("this better be the last one", 5)

	assert.NoError(t, store.SetBasis([]fdv2proto.Event{}, selector1, false))
	assert.Equal(t, selector1, store.Selector())

	assert.NoError(t, store.SetBasis([]fdv2proto.Event{}, selector2, false))
	assert.Equal(t, selector2, store.Selector())

	assert.NoError(t, store.ApplyDelta([]fdv2proto.Event{}, selector3, false))
	assert.Equal(t, selector3, store.Selector())

	assert.NoError(t, store.ApplyDelta([]fdv2proto.Event{}, selector4, false))
	assert.Equal(t, selector4, store.Selector())

	assert.NoError(t, store.Commit())
	assert.Equal(t, selector4, store.Selector())

	assert.NoError(t, store.SetBasis([]fdv2proto.Event{}, selector5, false))
}

func TestStore_Concurrency(t *testing.T) {
	t.Run("methods using the active store", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()

		var wg sync.WaitGroup

		run := func(f func()) {
			wg.Add(1)
			defer wg.Done()
			for i := 0; i < 100; i++ {
				f()
				time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
			}
		}

		go run(func() {
			_, _ = store.Get(ldstoreimpl.Features(), "foo")
		})
		go run(func() {
			_, _ = store.GetAll(ldstoreimpl.Features())
		})
		go run(func() {
			_ = store.GetDataStoreStatusProvider()
		})
		go run(func() {
			_ = store.IsInitialized()
		})
		go run(func() {
			_ = store.SetBasis([]fdv2proto.Event{}, fdv2proto.NoSelector(), true)
		})
		go run(func() {
			_ = store.ApplyDelta([]fdv2proto.Event{}, fdv2proto.NoSelector(), true)
		})
		go run(func() {
			_ = store.Selector()
		})
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

// This matcher is required instead of calling ElementsMatch directly on two slices of collections because
// the order of the collections, or the order within each collection, is not defined.
func requireCollectionsMatch(t *testing.T, expected []ldstoretypes.Collection, actual []ldstoretypes.Collection) {
	require.Equal(t, len(expected), len(actual))
	for _, expectedCollection := range expected {
		for _, actualCollection := range actual {
			if expectedCollection.Kind == actualCollection.Kind {
				require.ElementsMatch(t, expectedCollection.Items, actualCollection.Items)
				break
			}
		}
	}
}
