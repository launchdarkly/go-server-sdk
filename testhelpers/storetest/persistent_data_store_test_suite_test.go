package storetest

import (
	"errors"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-test-helpers/v3/testbox"

	"github.com/stretchr/testify/assert"
)

// This verifies that the PersistentDataStoreTestSuite tests behave as expected as long as the
// PersistentDataStore implementation behaves as expected, so we can distinguish between flaws in the
// implementations and flaws in the test logic.
//
// PersistentDataStore implementations may be able to persist the version and deleted state as metadata
// separate from the serialized item string; or they may not, in which case a little extra parsing is
// necessary. MockPersistentDataStore is able to simulate both of these scenarios, and we test both here.

type mockStoreFactory struct {
	db                  *mocks.MockDatabaseInstance
	prefix              string
	persistOnlyAsString bool
	fakeError           error
}

func (f mockStoreFactory) Build(context subsystems.ClientContext) (subsystems.PersistentDataStore, error) {
	store := mocks.NewMockPersistentDataStoreWithPrefix(f.db, f.prefix)
	store.SetPersistOnlyAsString(f.persistOnlyAsString)
	store.SetFakeError(f.fakeError)
	return store, nil
}

func TestPersistentDataStoreTestSuite(t *testing.T) {
	db := mocks.NewMockDatabaseInstance()

	baseSuite := func(persistOnlyAsString bool, fakeError error) *PersistentDataStoreTestSuite {
		return NewPersistentDataStoreTestSuite(
			func(prefix string) subsystems.ComponentConfigurer[subsystems.PersistentDataStore] {
				return mockStoreFactory{db, prefix, persistOnlyAsString, fakeError}
			},
			func(prefix string) error {
				db.Clear(prefix)
				return nil
			},
		)
	}

	runTests := func(t *testing.T, persistOnlyAsString bool) {
		baseSuite(persistOnlyAsString, nil).
			ConcurrentModificationHook(
				func(store subsystems.PersistentDataStore, hook func()) {
					store.(*mocks.MockPersistentDataStore).SetTestTxHook(hook)
				}).
			Run(t)
	}

	t.Run("with metadata stored separately from serialized item", func(t *testing.T) {
		runTests(t, false)
	})

	t.Run("with metadata stored only in serialized item", func(t *testing.T) {
		runTests(t, true)
	})

	t.Run("causing deliberate errors makes tests fail", func(t *testing.T) {
		fakeError := errors.New("sorry")
		s := baseSuite(false, fakeError)
		r := testbox.SandboxTest(s.runInternal)
		assert.True(t, r.Failed, "test should have failed")
	})

	t.Run("ErrorStoreFactory test for deliberate errors", func(t *testing.T) {
		fakeError := errors.New("sorry")
		s := baseSuite(false, nil).
			ErrorStoreFactory(
				mockStoreFactory{db, "errorprefix", false, fakeError},
				nil,
			)
		s.Run(t)
	})

	t.Run("ErrorStoreFactory test calls error validator", func(t *testing.T) {
		fakeError := errors.New("sorry")
		called := false
		s := baseSuite(false, nil).
			ErrorStoreFactory(
				mockStoreFactory{db, "errorprefix", false, fakeError},
				func(t assert.TestingT, err error) {
					called = true
					assert.Equal(t, fakeError, err)
				},
			)
		s.includeBaseTests = false
		s.Run(t)
		assert.True(t, called)
	})

	t.Run("ErrorStoreFactory test fails if error validator fails", func(t *testing.T) {
		fakeError := errors.New("sorry")
		s := baseSuite(false, nil).
			ErrorStoreFactory(
				mockStoreFactory{db, "errorprefix", false, fakeError},
				func(t assert.TestingT, err error) {
					assert.NotEqual(t, fakeError, err)
				},
			)
		s.includeBaseTests = false
		r := testbox.SandboxTest(s.runInternal)
		assert.True(t, r.Failed, "test should have failed")
	})
}
