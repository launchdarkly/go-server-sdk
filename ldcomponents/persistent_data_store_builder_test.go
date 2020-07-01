package ldcomponents

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

func TestPersistentDataStoreBuilder(t *testing.T) {
	t.Run("factory", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)
		assert.Equal(t, pdsf, f.persistentDataStoreFactory)
	})

	t.Run("calls factory", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		pdsf.store = sharedtest.NewMockPersistentDataStore()
		f := PersistentDataStore(pdsf)

		context := sharedtest.NewSimpleTestContext("")
		broadcaster := internal.NewDataStoreStatusBroadcaster()
		dataStoreUpdates := datastore.NewDataStoreUpdatesImpl(broadcaster)

		store, err := f.CreateDataStore(context, dataStoreUpdates)
		assert.NoError(t, err)
		require.NotNil(t, store)
		_ = store.Close()
		assert.Equal(t, context, pdsf.receivedContext)

		pdsf.store = nil
		pdsf.fakeError = errors.New("sorry")

		store, err = f.CreateDataStore(context, nil)
		assert.Equal(t, pdsf.fakeError, err)
		assert.Nil(t, store)
	})

	t.Run("CacheTime", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)

		f.CacheTime(time.Hour)
		assert.Equal(t, time.Hour, f.cacheTTL)
	})

	t.Run("CacheSeconds", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)

		f.CacheSeconds(44)
		assert.Equal(t, 44*time.Second, f.cacheTTL)
	})

	t.Run("CacheForever", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)

		f.CacheForever()
		assert.Equal(t, -1*time.Millisecond, f.cacheTTL)
	})

	t.Run("NoCaching", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)

		f.NoCaching()
		assert.Equal(t, time.Duration(0), f.cacheTTL)
	})

	t.Run("diagnostic description", func(t *testing.T) {
		f1 := PersistentDataStore(&mockPersistentDataStoreFactory{})
		assert.Equal(t, ldvalue.String("custom"), f1.DescribeConfiguration())

		f2 := PersistentDataStore(&mockPersistentDataStoreFactoryWithDescription{ldvalue.String("MyDatabase")})
		assert.Equal(t, ldvalue.String("MyDatabase"), f2.DescribeConfiguration())
	})
}

type mockPersistentDataStoreFactory struct {
	store           interfaces.PersistentDataStore
	fakeError       error
	receivedContext interfaces.ClientContext
}

func (m *mockPersistentDataStoreFactory) CreatePersistentDataStore(
	context interfaces.ClientContext,
) (interfaces.PersistentDataStore, error) {
	m.receivedContext = context
	return m.store, m.fakeError
}

type mockPersistentDataStoreFactoryWithDescription struct {
	description ldvalue.Value
}

func (m *mockPersistentDataStoreFactoryWithDescription) CreatePersistentDataStore(
	context interfaces.ClientContext,
) (interfaces.PersistentDataStore, error) {
	return nil, nil
}

func (m *mockPersistentDataStoreFactoryWithDescription) DescribeConfiguration() ldvalue.Value {
	return m.description
}
