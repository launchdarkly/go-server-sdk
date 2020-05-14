package sharedtest

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// This verifies that the PersistentDataStoreTestSuite tests behave as expected as long as the
// PersistentDataStore implementation behaves as expected, so we can distinguish between flaws in the
// implementations and flaws in the test logic.
//
// PersistentDataStore implementations may be able to persist the version and deleted state as metadata
// separate from the serialized item string; or they may not, in which case a little extra parsing is
// necessary. MockPersistentDataStore is able to simulate both of these scenarios, and we test both here.

type mockStoreFactory struct {
	db                  *mockDatabaseInstance
	prefix              string
	persistOnlyAsString bool
}

func (f mockStoreFactory) CreatePersistentDataStore(context interfaces.ClientContext) (interfaces.PersistentDataStore, error) {
	store := newMockPersistentDataStoreWithPrefix(f.db, f.prefix)
	store.persistOnlyAsString = f.persistOnlyAsString
	return store, nil
}

func TestPersistentDataStoreTestSuite(t *testing.T) {
	db := newMockDatabaseInstance()

	runTests := func(t *testing.T, persistOnlyAsString bool) {
		NewPersistentDataStoreTestSuite(
			func(prefix string) interfaces.PersistentDataStoreFactory {
				return mockStoreFactory{db, prefix, persistOnlyAsString}
			},
			func(prefix string) error {
				db.Clear(prefix)
				return nil
			},
		).ConcurrentModificationHook(
			func(store interfaces.PersistentDataStore, hook func()) {
				store.(*MockPersistentDataStore).testTxHook = hook
			},
		).Run(t)
	}

	t.Run("with metadata stored separately from serialized item", func(t *testing.T) {
		runTests(t, false)
	})

	t.Run("with metadata stored only in serialized item", func(t *testing.T) {
		runTests(t, true)
	})
}
