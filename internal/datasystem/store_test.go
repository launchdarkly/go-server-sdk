package datasystem

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v4/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v4/ldlogtest"
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
