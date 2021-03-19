package storetest

import (
	"reflect"
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers"

	"github.com/launchdarkly/go-test-helpers/v2/testbox"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fakeUserHash = "userhash"

// UnboundedSegmentStoreTestSuite provides a configurable test suite for all implementations of
// UnboundedSegmentStore.
type UnboundedSegmentStoreTestSuite struct {
	storeFactoryFn func(string) interfaces.UnboundedSegmentStoreFactory
	clearDataFn    func(string) error
	setMetadataFn  func(string, interfaces.UnboundedSegmentStoreMetadata) error
	setSegmentsFn  func(prefix, userHashKey string, included []string, excluded []string) error
}

// NewUnboundedSegmentStoreTestSuite creates an UnboundedSegmentStoreTestSuite for testing some
// implementation of UnboundedSegmentStore.
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
// The setMetadataFn and setSegmentsFn parameters are functions for populating the database.
func NewUnboundedSegmentStoreTestSuite(
	storeFactoryFn func(prefix string) interfaces.UnboundedSegmentStoreFactory,
	clearDataFn func(prefix string) error,
	setMetadataFn func(prefix string, metadata interfaces.UnboundedSegmentStoreMetadata) error,
	setSegmentsFn func(prefix string, userHashKey string, included []string, excluded []string) error,
) *UnboundedSegmentStoreTestSuite {
	return &UnboundedSegmentStoreTestSuite{
		storeFactoryFn: storeFactoryFn,
		clearDataFn:    clearDataFn,
		setMetadataFn:  setMetadataFn,
		setSegmentsFn:  setSegmentsFn,
	}
}

// Run runs the configured test suite.
func (s *UnboundedSegmentStoreTestSuite) Run(t *testing.T) {
	s.runInternal(testbox.RealTest(t))
}

func (s *UnboundedSegmentStoreTestSuite) runInternal(t testbox.TestingT) {
	t.Run("GetMetadata", s.runMetadataTests)
	t.Run("GetUserMembership", s.runUserMembershipTests)
}

func (s *UnboundedSegmentStoreTestSuite) runMetadataTests(t testbox.TestingT) {
	t.Run("valid value", func(t testbox.TestingT) {
		expected := interfaces.UnboundedSegmentStoreMetadata{LastUpToDate: ldtime.UnixMillisecondTime(1234567890)}

		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			require.NoError(t, s.setMetadataFn("", expected))

			meta, err := store.GetMetadata()
			require.NoError(t, err)
			assert.Equal(t, expected, meta)
		})
	})

	t.Run("no value", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			_, err := store.GetMetadata()
			require.Error(t, err)
		})
	})
}

func (s *UnboundedSegmentStoreTestSuite) runUserMembershipTests(t testbox.TestingT) {
	t.Run("not found", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			um, err := store.GetUserMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, nil, nil, um)
		})
	})

	t.Run("includes only", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, []string{"key1", "key2"}, nil))

			um, err := store.GetUserMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, []string{"key1", "key2"}, nil, um)
		})
	})

	t.Run("excludes only", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, nil, []string{"key1", "key2"}))

			um, err := store.GetUserMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, nil, []string{"key1", "key2"}, um)
		})
	})

	t.Run("includes and excludes", func(t testbox.TestingT) {
		s.withStoreAndEmptyData(t, func(store interfaces.UnboundedSegmentStore) {
			require.NoError(t, s.setSegmentsFn("", fakeUserHash, []string{"key1", "key2"}, []string{"key2", "key3"}))
			// key1 is included; key2 is included and excluded, therefore it's included; key3 is excluded

			um, err := store.GetUserMembership(fakeUserHash)
			require.NoError(t, err)
			assertEqualMembership(t, []string{"key1", "key2"}, []string{"key3"}, um)
		})
	})
}

func (s *UnboundedSegmentStoreTestSuite) withStoreAndEmptyData(
	t require.TestingT,
	action func(interfaces.UnboundedSegmentStore),
) {
	require.NoError(t, s.clearDataFn(""))

	store, err := s.storeFactoryFn("").CreateUnboundedSegmentStore(testhelpers.NewSimpleClientContext(""))
	require.NoError(t, err)
	defer func() {
		_ = store.Close()
	}()

	action(store)
}

func assertEqualMembership(
	t assert.TestingT,
	expectedIncludes []string,
	expectedExcludes []string,
	actual interfaces.UnboundedSegmentMembership,
) {
	// Most store implementations should use our helper types from ldstoreimpl. If they do, then we
	// can do an exact equality test. If they don't, then we'll just check that they include/exclude
	// the right keys (which isn't quite as good because we can't prove that they don't also have
	// other unwanted keys).
	expected := ldstoreimpl.NewUnboundedSegmentMembershipFromSegmentRefs(expectedIncludes, expectedExcludes)
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
