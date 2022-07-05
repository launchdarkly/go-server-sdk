package storetest

import (
	"reflect"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v6/testhelpers"

	"github.com/launchdarkly/go-test-helpers/v2/testbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeUserHash = "userhash"

// BigSegmentStoreTestSuite provides a configurable test suite for all implementations of
// BigSegmentStore.
type BigSegmentStoreTestSuite struct {
	storeFactoryFn func(string) subsystems.BigSegmentStoreFactory
	clearDataFn    func(string) error
	setMetadataFn  func(string, subsystems.BigSegmentStoreMetadata) error
	setSegmentsFn  func(prefix, userHashKey string, included []string, excluded []string) error
}

// NewBigSegmentStoreTestSuite creates an BigSegmentStoreTestSuite for testing some
// implementation of BigSegmentStore.
//
// The storeFactoryFn parameter is a function that takes a prefix string and returns a configured
// factory for this data store type (for instance, ldredis.DataStore().Prefix(prefix)). If the
// prefix string is "", it should use the default prefix defined by the data store implementation.
// The factory must include any necessary configuration that may be appropriate for the test
// environment (for instance, pointing it to a database instance that has been set up for the
// tests).
//
// The clearDataFn parameter is a function that takes a prefix string and deletes any existing
// data that may exist in the database corresponding to that prefix.
//
// The setMetadataFn and setSegmentsFn parameters are functions for populating the database. The
// string slices passed to setSegmentsFn are lists of segment references in the same format used
// by BigSegmentMembership, and should be used as-is by the store.
func NewBigSegmentStoreTestSuite(
	storeFactoryFn func(prefix string) subsystems.BigSegmentStoreFactory,
	clearDataFn func(prefix string) error,
	setMetadataFn func(prefix string, metadata subsystems.BigSegmentStoreMetadata) error,
	setSegmentsFn func(prefix string, userHashKey string, included []string, excluded []string) error,
) *BigSegmentStoreTestSuite {
	return &BigSegmentStoreTestSuite{
		storeFactoryFn: storeFactoryFn,
		clearDataFn:    clearDataFn,
		setMetadataFn:  setMetadataFn,
		setSegmentsFn:  setSegmentsFn,
	}
}

// Run runs the configured test suite.
func (s *BigSegmentStoreTestSuite) Run(t *testing.T) {
	s.runInternal(testbox.RealTest(t))
}

func (s *BigSegmentStoreTestSuite) runInternal(t testbox.TestingT) {
	t.Run("GetMetadata", s.runMetadataTests)
	t.Run("GetMembership", s.runMembershipTests)
}

func (s *BigSegmentStoreTestSuite) runMetadataTests(t testbox.TestingT) {
	t.Run("valid value", func(t testbox.TestingT) {
		expected := subsystems.BigSegmentStoreMetadata{LastUpToDate: ldtime.UnixMillisecondTime(1234567890)}

		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			require.NoError(t, s.setMetadataFn("", expected))

			meta, err := store.GetMetadata()
			require.NoError(t, err)
			assert.Equal(t, expected, meta)
		})
	})

	t.Run("no value", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			_, err := store.GetMetadata()
			require.Error(t, err)
		})
	})
}

func (s *BigSegmentStoreTestSuite) runMembershipTests(t testbox.TestingT) {
	t.Run("not found", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			um, err := store.GetMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, nil, nil, um)
		})
	})

	t.Run("includes only", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, []string{"key1", "key2"}, nil))

			um, err := store.GetMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, []string{"key1", "key2"}, nil, um)
		})
	})

	t.Run("excludes only", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, nil, []string{"key1", "key2"}))

			um, err := store.GetMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, nil, []string{"key1", "key2"}, um)
		})
	})

	t.Run("includes and excludes", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store subsystems.BigSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, []string{"key1", "key2"}, []string{"key2", "key3"}))
			// key1 is included; key2 is included and excluded, therefore it's included; key3 is excluded

			um, err := store.GetMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, []string{"key1", "key2"}, []string{"key3"}, um)
		})
	})
}

func (s *BigSegmentStoreTestSuite) withStoreAndEmptyData(
	t testbox.TestingT,
	action func(subsystems.BigSegmentStore),
) {
	require.NoError(t, s.clearDataFn(""))

	testhelpers.WithMockLoggingContext(t, func(context subsystems.ClientContext) {
		store, err := s.storeFactoryFn("").CreateBigSegmentStore(context)
		require.NoError(t, err)
		defer func() {
			_ = store.Close()
		}()

		action(store)
	})
}

func assertEqualMembership(
	t assert.TestingT,
	expectedIncludes []string,
	expectedExcludes []string,
	actual subsystems.BigSegmentMembership,
) {
	// Most store implementations should use our helper types from ldstoreimpl. If they do, then we
	// can do an exact equality test. If they don't, then we'll just check that they include/exclude
	// the right keys (which isn't quite as good because we can't prove that they don't also have
	// other unwanted keys).
	expected := ldstoreimpl.NewBigSegmentMembershipFromSegmentRefs(expectedIncludes, expectedExcludes)
	if reflect.TypeOf(actual) == reflect.TypeOf(expected) {
		assert.Equal(t, expected, actual)
	} else {
		for _, inc := range expectedIncludes {
			assert.Equal(t, ldvalue.NewOptionalBool(true), actual.CheckMembership(inc), "for key %q", inc)
		}
		for _, exc := range expectedIncludes {
			assert.Equal(t, ldvalue.NewOptionalBool(false), actual.CheckMembership(exc), "for key %q", exc)
		}
		// here's a key we'll never use, just to make sure it's not answering "yes" to everything
		assert.Equal(t, ldvalue.OptionalBool{}, actual.CheckMembership("unused-key"), `for key "unused-key"`)
	}
}
