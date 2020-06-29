package storetest

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	sh "gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

// This verifies that the PersistentDataStoreTestSuite tests behave as expected as long as the
// PersistentDataStore implementation behaves as expected, so we can distinguish between flaws in the
// implementations and flaws in the test logic.
//
// PersistentDataStore implementations may be able to persist the version and deleted state as metadata
// separate from the serialized item string; or they may not, in which case a little extra parsing is
// necessary. MockPersistentDataStore is able to simulate both of these scenarios, and we test both here.

type mockStoreFactory struct {
	db                  *sharedtest.MockDatabaseInstance
	prefix              string
	persistOnlyAsString bool
	fakeError           error
}

func (f mockStoreFactory) CreatePersistentDataStore(context interfaces.ClientContext) (interfaces.PersistentDataStore, error) {
	store := sh.NewMockPersistentDataStoreWithPrefix(f.db, f.prefix)
	store.SetPersistOnlyAsString(f.persistOnlyAsString)
	store.SetFakeError(f.fakeError)
	return store, nil
}

func TestPersistentDataStoreTestSuite(t *testing.T) {
	db := sh.NewMockDatabaseInstance()

	baseSuite := func(persistOnlyAsString bool) *PersistentDataStoreTestSuite {
		return NewPersistentDataStoreTestSuite(
			func(prefix string) interfaces.PersistentDataStoreFactory {
				return mockStoreFactory{db, prefix, persistOnlyAsString, nil}
			},
			func(prefix string) error {
				db.Clear(prefix)
				return nil
			},
		).AlwaysRun(true)
	}

	runTests := func(t *testing.T, persistOnlyAsString bool) {
		baseSuite(persistOnlyAsString).
			ConcurrentModificationHook(
				func(store interfaces.PersistentDataStore, hook func()) {
					store.(*sh.MockPersistentDataStore).SetTestTxHook(hook)
				}).
			Run(t)
	}

	t.Run("with metadata stored separately from serialized item", func(t *testing.T) {
		runTests(t, false)
	})

	t.Run("with metadata stored only in serialized item", func(t *testing.T) {
		runTests(t, true)
	})

	t.Run("with deliberate errors and error validator", func(t *testing.T) {
		fakeError := errors.New("sorry")
		s := baseSuite(false).
			ErrorStoreFactory(
				mockStoreFactory{db, "errorprefix", false, fakeError},
				func(t assert.TestingT, err error) {
					assert.Equal(t, fakeError, err)
				},
			)
		s.includeBaseTests = false
		s.Run(t)
	})

	t.Run("with deliberate errors and no error validator", func(t *testing.T) {
		fakeError := errors.New("sorry")
		s := baseSuite(false).
			ErrorStoreFactory(
				mockStoreFactory{db, "errorprefix", false, fakeError},
				nil,
			)
		s.includeBaseTests = false
		s.Run(t)
	})

	t.Run("skip if LD_SKIP_DATABASE_TESTS is set", func(t *testing.T) {
		varName := "LD_SKIP_DATABASE_TESTS"
		oldValue := os.Getenv(varName)
		os.Setenv(varName, "1")
		defer os.Setenv(varName, oldValue)
		baseSuite(false).AlwaysRun(false).Run(t)
		assert.Fail(t, "should not have reached this line (test should have been skipped)")
	})
}
