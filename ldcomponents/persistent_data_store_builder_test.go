package ldcomponents

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentDataStoreBuilder(t *testing.T) {
	t.Run("factory", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		f := PersistentDataStore(pdsf)
		assert.Equal(t, pdsf, f.persistentDataStoreFactory)
	})

	t.Run("calls factory", func(t *testing.T) {
		pdsf := &mockPersistentDataStoreFactory{}
		pdsf.store = mocks.NewMockPersistentDataStore()
		f := PersistentDataStore(pdsf)

		logConfig := subsystems.LoggingConfiguration{Loggers: ldlog.NewDisabledLoggers()}
		clientContext := sharedtest.NewTestContext("", nil, &logConfig)
		broadcaster := internal.NewBroadcaster[interfaces.DataStoreStatus]()
		clientContext.DataStoreUpdateSink = datastore.NewDataStoreUpdateSinkImpl(broadcaster)

		store, err := f.Build(clientContext)
		assert.NoError(t, err)
		require.NotNil(t, store)
		_ = store.Close()
		assert.Equal(t, clientContext.GetLogging(), pdsf.receivedContext.GetLogging())

		pdsf.store = nil
		pdsf.fakeError = errors.New("sorry")

		store, err = f.Build(clientContext)
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
		assert.Equal(t, ldvalue.String("custom"), f1.DescribeConfiguration(basicClientContext()))

		f2 := PersistentDataStore(&mockPersistentDataStoreFactoryWithDescription{ldvalue.String("MyDatabase")})
		assert.Equal(t, ldvalue.String("MyDatabase"), f2.DescribeConfiguration(basicClientContext()))
	})
}

type mockPersistentDataStoreFactory struct {
	store           subsystems.PersistentDataStore
	fakeError       error
	receivedContext subsystems.ClientContext
}

func (m *mockPersistentDataStoreFactory) Build(
	context subsystems.ClientContext,
) (subsystems.PersistentDataStore, error) {
	m.receivedContext = context
	return m.store, m.fakeError
}

type mockPersistentDataStoreFactoryWithDescription struct {
	description ldvalue.Value
}

func (m *mockPersistentDataStoreFactoryWithDescription) Build(
	context subsystems.ClientContext,
) (subsystems.PersistentDataStore, error) {
	return nil, nil
}

func (m *mockPersistentDataStoreFactoryWithDescription) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return m.description
}
