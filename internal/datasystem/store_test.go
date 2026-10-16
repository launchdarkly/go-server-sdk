package datasystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/overrides"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

func TestStore_New(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	assert.NoError(t, store.Close())
}

func TestStore_NoSelector(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	defer store.Close()
	assert.Equal(t, subsystems.NoSelector(), store.Selector())
}

func TestStore_NoPersistence_NewStore_IsNotInitialized(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	defer store.Close()
	assert.False(t, store.IsInitialized())
}

func TestStore_NoPersistence_MemoryStore_IsInitialized(t *testing.T) {
	v1 := subsystems.NewSelector("foo", 1)
	none := subsystems.NoSelector()
	tests := []struct {
		name     string
		selector subsystems.Selector
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
			flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
			changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
			store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
			defer store.Close()

			store.Apply(*subsystems.NewChangeSetBuilder().Empty(tt.selector), tt.persist)
			assert.True(t, store.IsInitialized())
		})
	}
}

func MustMarshal(model any) json.RawMessage {
	data, err := json.Marshal(model)
	if err != nil {
		panic(err)
	}
	return data
}

func MinimalFlag(key string, version int) json.RawMessage {
	return []byte(fmt.Sprintf(`{"key":"`+key+`","version": %v}`, version))
}

func MinimalSegment(key string, version int) json.RawMessage {
	return []byte(fmt.Sprintf(`{"key":"`+key+`","version": %v}`, version))
}

func TestStore_Commit(t *testing.T) {
	t.Run("absence of persistent store doesn't cause error when committing", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
		defer store.Close()
		assert.NoError(t, store.Commit())
	})

	t.Run("persist-marked memory items are copied to persistent store in r/w mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		// isDown causes the fake to reject updates (until flipped to false).
		spy := &fakeStore{isDown: true}
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()

		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		builder.AddPut(subsystems.SegmentKind, "bar", 2, MinimalSegment("bar", 2))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		// OK: basically we need to match up the JSON with the FlagBuilder stuff for this to work, a naive marshal won't work.
		// The original data system PR has some infra for this. Maybe bring it in in this pr.
		output := []ldstoretypes.Collection{
			{
				Kind: ldstoreimpl.Features(),
				Items: []ldstoretypes.KeyedItemDescriptor{
					{Key: "foo", Item: sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("foo").Version(1).Build())},
				},
			},
			{
				Kind: ldstoreimpl.Segments(),
				Items: []ldstoretypes.KeyedItemDescriptor{
					{Key: "bar", Item: sharedtest.SegmentDescriptor(ldbuilders.NewSegmentBuilder("bar").Version(2).Build())},
				},
			},
		}

		// There should be an error since writing to the store will fail.
		store.Apply(*changeSet, true)

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
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		builder.AddPut(subsystems.SegmentKind, "bar", 2, MinimalSegment("bar", 2))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		store.Apply(*changeSet, false)

		// Since Apply will immediately mirror the data if persist == true, we can check this is empty now.
		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		// Commit should be a no-op. This tests that the persist status was saved.
		assert.Empty(t, spy.initPayload)
	})

	t.Run("persist-marked memory items are not copied to persistent store in r-only mode", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		// The fake should accept updates.
		spy := &fakeStore{isDown: false}
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).WithPersistence(spy, subsystems.DataStoreModeRead, nil)
		defer store.Close()

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		builder.AddPut(subsystems.SegmentKind, "bar", 2, MinimalSegment("bar", 2))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		// Even though persist is true, the store was marked as read-only, so it shouldn't be written to.
		store.Apply(*changeSet, true)

		require.Empty(t, spy.initPayload)

		require.NoError(t, store.Commit())

		// Same with commit.
		assert.Empty(t, spy.initPayload)
	})
}

func TestStore_GetActive(t *testing.T) {
	t.Run("memory store is active if no persistent store configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
		defer store.Close()
		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, foo, ldstoretypes.ItemDescriptor{}.NotFound())

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		store.Apply(*changeSet, false)

		foo, err = store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})

	t.Run("persistent store is active if configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()

		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).WithPersistence(&fakeStore{}, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		_, err := store.Get(ldstoreimpl.Features(), "foo")

		// The fakeStore should return a specific error when Get is called.
		assert.Equal(t, errImAPersistentStore, err)
	})

	t.Run("active store swaps from persistent to memory", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).WithPersistence(&fakeStore{}, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		// Before there's any data, if we call Get the persistent store should be accessed.
		_, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.Equal(t, errImAPersistentStore, err)

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)
		store.Apply(*changeSet, false)

		// Now that there's memory data, the persistent store should no longer be accessed.
		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})
}

func TestStore_SelectorIsRemembered(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	defer store.Close()

	selector1 := subsystems.NewSelector("foo", 1)
	selector2 := subsystems.NewSelector("bar", 2)
	selector3 := subsystems.NewSelector("baz", 3)
	selector4 := subsystems.NewSelector("qux", 4)

	builder := subsystems.NewChangeSetBuilder()
	builder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{
			ID:     "state",
			Target: 1,
			Code:   subsystems.IntentTransferFull,
			Reason: "cant-catchup",
		},
	})
	changeSet, err := builder.Finish(selector1)
	require.NoError(t, err)
	store.Apply(*changeSet, false)
	assert.Equal(t, selector1, store.Selector())

	changeSet, err = builder.Finish(selector2)
	require.NoError(t, err)
	store.Apply(*changeSet, false)
	assert.Equal(t, selector2, store.Selector())

	changeSet, err = builder.Finish(selector3)
	require.NoError(t, err)
	store.Apply(*changeSet, false)
	assert.Equal(t, selector3, store.Selector())

	changeSet, err = builder.Finish(selector4)
	require.NoError(t, err)
	store.Apply(*changeSet, false)
	assert.Equal(t, selector4, store.Selector())

	assert.NoError(t, store.Commit())
	assert.Equal(t, selector4, store.Selector())
}

func TestStore_Concurrency(t *testing.T) {
	t.Run("methods using the active store", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
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
			store.Apply(*subsystems.NewChangeSetBuilder().Empty(subsystems.NoSelector()), true)
		})
		go run(func() {
			builder := subsystems.NewChangeSetBuilder()
			builder.Start(subsystems.ServerIntent{
				Payload: subsystems.Payload{
					ID:     "state",
					Target: 1,
					Code:   subsystems.IntentTransferChanges,
					Reason: "stale",
				},
			})
			changeSet, err := builder.Finish(subsystems.NoSelector())
			require.NoError(t, err)

			store.Apply(*changeSet, true)
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

// fakeCacheDroppingStore is a fakeStore that also satisfies interface{ DropCache() },
// so the Store coordinator's WithPersistence captures it as a cacheDropper.
type fakeCacheDroppingStore struct {
	fakeStore
	dropCacheCalls int
}

func (f *fakeCacheDroppingStore) DropCache() {
	f.dropCacheCalls++
}

// fakeExportingStore is a fakeStore that also accepts a CacheExporter, as the wrapper does.
type fakeExportingStore struct {
	fakeStore
	exporter datastore.CacheExporter
}

func (f *fakeExportingStore) SetCacheExporter(exporter datastore.CacheExporter) {
	f.exporter = exporter
}

func TestStore_CacheExporter(t *testing.T) {
	newStoreWithPersistence := func(spy *fakeExportingStore, mode subsystems.DataStoreMode) *Store {
		logCapture := ldlogtest.NewMockLog()
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		return NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).
			WithPersistence(spy, mode, nil)
	}

	t.Run("the store registers itself with a read-write persistent store", func(t *testing.T) {
		spy := &fakeExportingStore{}
		store := newStoreWithPersistence(spy, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		assert.Same(t, store, spy.exporter)
	})

	t.Run("the store does not register itself with a read-only persistent store", func(t *testing.T) {
		spy := &fakeExportingStore{}
		store := newStoreWithPersistence(spy, subsystems.DataStoreModeRead)
		defer store.Close()

		assert.Nil(t, spy.exporter)
	})

	t.Run("ExportTo passes the data held in the memory store to the write", func(t *testing.T) {
		store := newStoreWithPersistence(&fakeExportingStore{}, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		store.Apply(makeFlagChangeSet(t, subsystems.IntentTransferFull, []string{"foo"}), true)

		var written []ldstoretypes.Collection
		exported, err := store.ExportTo(func(data []ldstoretypes.Collection) error {
			written = data
			return nil
		})
		assert.True(t, exported)
		assert.NoError(t, err)
		requireCollectionsMatch(t, []ldstoretypes.Collection{
			{
				Kind: ldstoreimpl.Features(),
				Items: []ldstoretypes.KeyedItemDescriptor{
					{Key: "foo", Item: sharedtest.FlagDescriptor(ldbuilders.NewFlagBuilder("foo").Version(1).Build())},
				},
			},
		}, written)
	})

	t.Run("ExportTo does not write when the data must not be persisted", func(t *testing.T) {
		store := newStoreWithPersistence(&fakeExportingStore{}, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		store.Apply(makeFlagChangeSet(t, subsystems.IntentTransferFull, []string{"foo"}), false)

		exported, err := store.ExportTo(func(data []ldstoretypes.Collection) error {
			assert.Fail(t, "the store must not be written")
			return nil
		})
		assert.False(t, exported)
		assert.NoError(t, err)
	})

	t.Run("ExportTo returns the error from the write", func(t *testing.T) {
		store := newStoreWithPersistence(&fakeExportingStore{}, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		store.Apply(makeFlagChangeSet(t, subsystems.IntentTransferFull, []string{"foo"}), true)

		writeError := errors.New("store is down")
		exported, err := store.ExportTo(func(data []ldstoretypes.Collection) error { return writeError })
		assert.True(t, exported)
		assert.Equal(t, writeError, err)
	})

	// A persistent store must receive segments before the flags that reference them.
	t.Run("ExportTo writes segments before flags", func(t *testing.T) {
		store := newStoreWithPersistence(&fakeExportingStore{}, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{ID: "state", Target: 1, Code: subsystems.IntentTransferFull},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		builder.AddPut(subsystems.SegmentKind, "bar", 1, MinimalSegment("bar", 1))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)
		store.Apply(*changeSet, true)

		var written []ldstoretypes.Collection
		_, err = store.ExportTo(func(data []ldstoretypes.Collection) error {
			written = data
			return nil
		})
		require.NoError(t, err)
		require.Len(t, written, 2)
		assert.Equal(t, ldstoreimpl.Segments(), written[0].Kind)
		assert.Equal(t, ldstoreimpl.Features(), written[1].Kind)
	})

	// An update applied while the write is in progress would otherwise be overwritten by the
	// older data the write is still sending.
	t.Run("ExportTo blocks updates until the write completes", func(t *testing.T) {
		store := newStoreWithPersistence(&fakeExportingStore{}, subsystems.DataStoreModeReadWrite)
		defer store.Close()

		store.Apply(makeFlagChangeSet(t, subsystems.IntentTransferFull, []string{"foo"}), true)

		exported, err := store.ExportTo(func(data []ldstoretypes.Collection) error {
			if store.mu.TryLock() {
				store.mu.Unlock()
				assert.Fail(t, "an update could have been applied during the write")
			}
			return nil
		})
		assert.True(t, exported)
		assert.NoError(t, err)
	})
}

func TestStore_CacheDroppedAfterFullPayload(t *testing.T) {
	t.Run("DropCache is invoked once setBasis applies a full payload", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		spy := &fakeCacheDroppingStore{}
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).
			WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		assert.Equal(t, 0, spy.dropCacheCalls)

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		store.Apply(*changeSet, true)

		assert.Equal(t, 1, spy.dropCacheCalls, "DropCache should have been invoked once during setBasis")
	})

	t.Run("non-cache-dropping persistent stores are a no-op", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		// fakeStore does NOT implement interface{ DropCache() }.
		plain := &fakeStore{}
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).
			WithPersistence(plain, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
			},
		})
		builder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)

		require.NotPanics(t, func() {
			store.Apply(*changeSet, true)
		})
	})

	t.Run("DropCache is not invoked for delta updates", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		spy := &fakeCacheDroppingStore{}
		flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
		changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
		store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster).
			WithPersistence(spy, subsystems.DataStoreModeReadWrite, nil)
		defer store.Close()

		// First, prime the store with a full payload (setBasis) -- this should drop once.
		fullBuilder := subsystems.NewChangeSetBuilder()
		fullBuilder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 1,
				Code:   subsystems.IntentTransferFull,
			},
		})
		fullBuilder.AddPut(subsystems.FlagKind, "foo", 1, MinimalFlag("foo", 1))
		fullCS, err := fullBuilder.Finish(subsystems.NoSelector())
		require.NoError(t, err)
		store.Apply(*fullCS, true)
		require.Equal(t, 1, spy.dropCacheCalls)

		// Now apply a delta -- this should not invoke DropCache again.
		deltaBuilder := subsystems.NewChangeSetBuilder()
		deltaBuilder.Start(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "state",
				Target: 2,
				Code:   subsystems.IntentTransferChanges,
			},
		})
		deltaBuilder.AddPut(subsystems.FlagKind, "bar", 1, MinimalFlag("bar", 1))
		deltaCS, err := deltaBuilder.Finish(subsystems.NoSelector())
		require.NoError(t, err)
		store.Apply(*deltaCS, true)

		assert.Equal(t, 1, spy.dropCacheCalls, "DropCache should only be invoked from setBasis, not from applyDelta")
	})
}

// This matcher is required instead of calling ElementsMatch directly on two slices of collections because
// the order of the collections, or the order within each collection, is not defined.
func requireCollectionsMatch(t *testing.T, expected []ldstoretypes.Collection, actual []ldstoretypes.Collection) {
	t.Helper()
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

// makeFlagChangeSet builds a changeset with the given intent that puts one minimal
// flag per key.
func makeFlagChangeSet(t *testing.T, code subsystems.IntentCode, keys []string) subsystems.ChangeSet {
	t.Helper()
	builder := subsystems.NewChangeSetBuilder()
	builder.Start(subsystems.ServerIntent{
		Payload: subsystems.Payload{ID: "state", Target: 1, Code: code},
	})
	for _, key := range keys {
		builder.AddPut(subsystems.FlagKind, key, 1, MinimalFlag(key, 1))
	}
	changeSet, err := builder.Finish(subsystems.NoSelector())
	require.NoError(t, err)
	return *changeSet
}

func TestStore_ApplyCompletesWhileListenerReadsStore(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	defer store.Close()

	flagEvents := flagChangeBroadcaster.AddListener()
	defer flagChangeBroadcaster.RemoveListener(flagEvents)
	changeSetEvents := changeSetBroadcaster.AddListener()
	defer changeSetBroadcaster.RemoveListener(changeSetEvents)

	// The listener reads from the store before it takes the next event, like a flag
	// value change listener that evaluates a flag for each event.
	receivedFlagEvents := make(chan int, 1)
	go func() {
		count := 0
		for range flagEvents {
			_ = store.Selector()
			count++
		}
		receivedFlagEvents <- count
	}()

	applyBounded := func(changeSet subsystems.ChangeSet) {
		t.Helper()
		done := make(chan bool, 1)
		go func() { done <- store.Apply(changeSet, false) }()
		select {
		case applied := <-done:
			require.True(t, applied)
		case <-time.After(5 * time.Second):
			t.Fatal("Apply must return while a listener reads from the store")
		}
	}

	// Each changeset holds more flags than a listener channel can buffer, so delivery
	// requires the listener to drain events while Apply runs.
	fullKeys := make([]string, 0, 15)
	for i := 0; i < 15; i++ {
		fullKeys = append(fullKeys, fmt.Sprintf("full-flag-%d", i))
	}
	deltaKeys := make([]string, 0, 12)
	for i := 0; i < 12; i++ {
		deltaKeys = append(deltaKeys, fmt.Sprintf("delta-flag-%d", i))
	}

	// Both the full transfer and the delta transfer must complete.
	applyBounded(makeFlagChangeSet(t, subsystems.IntentTransferFull, fullKeys))
	applyBounded(makeFlagChangeSet(t, subsystems.IntentTransferChanges, deltaKeys))

	// Each applied changeset produces one changeset event.
	for i := 0; i < 2; i++ {
		select {
		case <-changeSetEvents:
		case <-time.After(5 * time.Second):
			t.Fatal("a changeset event must be delivered for each applied changeset")
		}
	}

	// Each affected flag produces one event; no events are dropped. Removing the
	// listener closes its channel, which ends the counting goroutine.
	flagChangeBroadcaster.RemoveListener(flagEvents)
	select {
	case count := <-receivedFlagEvents:
		assert.Equal(t, len(fullKeys)+len(deltaKeys), count)
	case <-time.After(5 * time.Second):
		t.Fatal("the flag event listener must observe the end of its channel")
	}

	// Close must complete after the events are delivered.
	closeDone := make(chan error, 1)
	go func() { closeDone <- store.Close() }()
	select {
	case err := <-closeDone:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Close must complete")
	}
}

// collectFlagChanges takes flag change events until none arrives for a short while.
func collectFlagChanges(ch <-chan interfaces.FlagChangeEvent) []string {
	keys := []string{}
	for {
		select {
		case event := <-ch:
			keys = append(keys, event.Key)
		case <-time.After(200 * time.Millisecond):
			sort.Strings(keys)
			return keys
		}
	}
}

func TestStore_LaunchDarklyChangeReachesOverrideDependents(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	flagChangeBroadcaster := internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	changeSetBroadcaster := internal.NewBroadcaster[subsystems.ChangeSet]()
	store := NewStore(logCapture.Loggers, flagChangeBroadcaster, changeSetBroadcaster)
	defer store.Close()
	flagEvents := flagChangeBroadcaster.AddListener()
	defer flagChangeBroadcaster.RemoveListener(flagEvents)

	// The override "overridden" matches the LaunchDarkly segment "segment1". The LaunchDarkly flag
	// "parent" has "overridden" as a prerequisite, so a change to the segment reaches parent through
	// the override entry: LaunchDarkly data -> override entry -> LaunchDarkly data.
	layer := overrides.NewLayer()
	overridden := ldbuilders.NewFlagBuilder("overridden").
		AddRule(ldbuilders.NewRuleBuilder().ID("rule").Variation(0).Clauses(ldbuilders.SegmentMatchClause("segment1"))).
		Build()
	layer.SetAll([]ldstoretypes.Collection{{
		Kind:  datakinds.Features,
		Items: []ldstoretypes.KeyedItemDescriptor{{Key: "overridden", Item: sharedtest.FlagDescriptor(overridden)}},
	}})
	store.setExternalDependencies(layer)

	put := func(kind subsystems.ObjectKind, key string, version int, item any) subsystems.PutObject {
		t.Helper()
		object, err := json.Marshal(item)
		require.NoError(t, err)
		return subsystems.PutObject{Kind: kind, Key: key, Version: version, Object: object}
	}
	applyPuts := func(code subsystems.IntentCode, target int, puts ...subsystems.PutObject) {
		t.Helper()
		builder := subsystems.NewChangeSetBuilder()
		builder.Start(subsystems.ServerIntent{Payload: subsystems.Payload{ID: "state", Target: target, Code: code}})
		for _, p := range puts {
			builder.AddPut(p.Kind, p.Key, p.Version, p.Object)
		}
		changeSet, err := builder.Finish(subsystems.NoSelector())
		require.NoError(t, err)
		require.True(t, store.Apply(*changeSet, false))
	}

	applyPuts(subsystems.IntentTransferFull, 1,
		put(subsystems.FlagKind, "parent", 1, ldbuilders.NewFlagBuilder("parent").Version(1).AddPrerequisite("overridden", 0).Build()),
		put(subsystems.SegmentKind, "segment1", 1, ldbuilders.NewSegmentBuilder("segment1").Version(1).Build()))
	// The initial data set counts every item as modified; the segment reaches the override.
	assert.Equal(t, []string{"overridden", "parent"}, collectFlagChanges(flagEvents))

	applyPuts(subsystems.IntentTransferChanges, 2,
		put(subsystems.SegmentKind, "segment1", 2, ldbuilders.NewSegmentBuilder("segment1").Version(2).Build()))
	assert.Equal(t, []string{"overridden", "parent"}, collectFlagChanges(flagEvents))

	// An item nothing references notifies no flag.
	applyPuts(subsystems.IntentTransferChanges, 3,
		put(subsystems.SegmentKind, "other", 1, ldbuilders.NewSegmentBuilder("other").Version(1).Build()))
	assert.Equal(t, []string{}, collectFlagChanges(flagEvents))
}
