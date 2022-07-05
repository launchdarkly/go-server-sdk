package storetest

import (
	"errors"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-test-helpers/v2/testbox"

	"github.com/stretchr/testify/assert"
)

// This verifies that the BigSegmentStoreTestSuite tests behave as expected as long as the
// BigSegmentStore implementation behaves as expected, so we can distinguish between flaws in the
// implementations and flaws in the test logic.

type mockSegmentStoreData struct {
	storesByPrefix        map[string]*mockSegmentStore
	overrideGetMetadata   func(*mockSegmentStore) (subsystems.BigSegmentStoreMetadata, error)
	overrideGetMembership func(*mockSegmentStore, string) (subsystems.BigSegmentMembership, error)
}

type mockSegmentStoreFactory struct {
	store *mockSegmentStore
}

type mockSegmentStore struct {
	owner    *mockSegmentStoreData
	prefix   string
	metadata *subsystems.BigSegmentStoreMetadata
	data     map[string]mockSegmentStoreKeys
}

type mockSegmentStoreKeys struct {
	included []string
	excluded []string
}

func (f mockSegmentStoreFactory) CreateBigSegmentStore(context subsystems.ClientContext) (subsystems.BigSegmentStore, error) {
	return f.store, nil
}

func (s *mockSegmentStore) Close() error { return nil }

func (s *mockSegmentStore) GetMetadata() (subsystems.BigSegmentStoreMetadata, error) {
	if s.owner.overrideGetMetadata != nil {
		return s.owner.overrideGetMetadata(s)
	}
	if s.metadata == nil {
		return subsystems.BigSegmentStoreMetadata{}, errors.New("not found")
	}
	return *s.metadata, nil
}

func (s *mockSegmentStore) GetMembership(contextHashKey string) (subsystems.BigSegmentMembership, error) {
	if s.owner.overrideGetMembership != nil {
		return s.owner.overrideGetMembership(s, contextHashKey)
	}
	keys := s.data[contextHashKey]
	return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(keys.included, keys.excluded), nil
}

func (d *mockSegmentStoreData) factory(prefix string) subsystems.BigSegmentStoreFactory {
	store := d.storesByPrefix[prefix]
	if store == nil {
		store = &mockSegmentStore{owner: d, data: make(map[string]mockSegmentStoreKeys)}
		if d.storesByPrefix == nil {
			d.storesByPrefix = make(map[string]*mockSegmentStore)
		}
		d.storesByPrefix[prefix] = store
	}
	return mockSegmentStoreFactory{store}
}

func (d *mockSegmentStoreData) clearData(prefix string) error {
	if store := d.storesByPrefix[prefix]; store != nil {
		store.metadata = nil
		store.data = make(map[string]mockSegmentStoreKeys)
	}
	return nil
}

func (d *mockSegmentStoreData) setMetadata(prefix string, metadata subsystems.BigSegmentStoreMetadata) error {
	if store := d.storesByPrefix[prefix]; store != nil {
		store.metadata = &metadata
		return nil
	}
	return errors.New("store not initialized for this prefix")
}

func (d *mockSegmentStoreData) setKeys(prefix, userHashKey string, included, excluded []string) error {
	if store := d.storesByPrefix[prefix]; store != nil {
		store.data[userHashKey] = mockSegmentStoreKeys{included, excluded}
		return nil
	}
	return errors.New("store not initialized for this prefix")
}

func TestBigSegmentStoreTestSuite(t *testing.T) {
	makeSuite := func(d *mockSegmentStoreData) *BigSegmentStoreTestSuite {
		return NewBigSegmentStoreTestSuite(d.factory, d.clearData, d.setMetadata, d.setKeys)
	}

	fakeError := errors.New("sorry")

	t.Run("tests pass with valid mock store", func(t *testing.T) {
		s := makeSuite(&mockSegmentStoreData{})
		s.Run(t)
	})

	t.Run("tests fail with malfunctioning store", func(t *testing.T) {
		shouldFail := func(t *testing.T, s *BigSegmentStoreTestSuite) {
			r := testbox.SandboxTest(s.runInternal)
			assert.True(t, r.Failed, "test should have failed")
		}

		t.Run("GetMetadata returns error", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMetadata: func(*mockSegmentStore) (subsystems.BigSegmentStoreMetadata, error) {
					return subsystems.BigSegmentStoreMetadata{}, fakeError
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetMetadata returns incorrect value", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMetadata: func(store *mockSegmentStore) (subsystems.BigSegmentStoreMetadata, error) {
					var metadata subsystems.BigSegmentStoreMetadata
					if store.metadata != nil {
						metadata = *store.metadata
					}
					metadata.LastUpToDate++
					return metadata, nil
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetUserMembership returns error", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMembership: func(*mockSegmentStore, string) (subsystems.BigSegmentMembership, error) {
					return nil, fakeError
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetUserMembership doesn't get included keys", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMembership: func(store *mockSegmentStore, userHashKey string) (subsystems.BigSegmentMembership, error) {
					keys := store.data[userHashKey]
					return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(keys.included, nil), nil
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetUserMembership doesn't get excluded keys", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMembership: func(store *mockSegmentStore, userHashKey string) (subsystems.BigSegmentMembership, error) {
					keys := store.data[userHashKey]
					return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(nil, keys.excluded), nil
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetUserMembership gets an extra included key", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMembership: func(store *mockSegmentStore, userHashKey string) (subsystems.BigSegmentMembership, error) {
					keys := store.data[userHashKey]
					return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(
						append(append([]string(nil), keys.included...), "unwanted-key"),
						keys.excluded,
					), nil
				},
			})
			shouldFail(t, s)
		})

		t.Run("GetUserMembership gets an extra excluded key", func(t *testing.T) {
			s := makeSuite(&mockSegmentStoreData{
				overrideGetMembership: func(store *mockSegmentStore, userHashKey string) (subsystems.BigSegmentMembership, error) {
					keys := store.data[userHashKey]
					return ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(
						keys.included,
						append(append([]string(nil), keys.excluded...), "unwanted-key"),
					), nil
				},
			})
			shouldFail(t, s)
		})
	})
}
