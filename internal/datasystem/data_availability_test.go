package datasystem

import (
	"context"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataAvailibilityAtLeast(t *testing.T) {
	assert.True(t, Refreshed.AtLeast(Refreshed))
	assert.True(t, Refreshed.AtLeast(Cached))
	assert.True(t, Refreshed.AtLeast(Defaults))

	assert.False(t, Cached.AtLeast(Refreshed))
	assert.True(t, Cached.AtLeast(Cached))
	assert.True(t, Cached.AtLeast(Defaults))

	assert.False(t, Defaults.AtLeast(Refreshed))
	assert.False(t, Defaults.AtLeast(Cached))
	assert.True(t, Defaults.AtLeast(Defaults))
}

func makeTestClientContext() *internal.ClientContextImpl {
	basicContext := sharedtest.NewSimpleTestContext(sharedtest.TestSDKKey)
	return &internal.ClientContextImpl{
		BasicClientContext: basicContext.(subsystems.BasicClientContext),
	}
}

func makeTestStore(initialized bool) subsystems.DataStore {
	store := datastore.NewInMemoryDataStore(ldlog.NewDisabledLoggers())
	if initialized {
		_ = store.Init([]ldstoretypes.Collection{})
	}
	return store
}

func TestFDv1DataAvailability(t *testing.T) {
	t.Run("offline mode", func(t *testing.T) {
		clientContext := makeTestClientContext()
		clientContext.Offline = true

		fdv1, err := NewFDv1(true, nil, nil, clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Defaults, fdv1.DataAvailability())
	})

	t.Run("LDD mode with initialized store", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			ldcomponents.ExternalUpdatesOnly(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Cached, fdv1.DataAvailability())
	})

	t.Run("LDD mode with uninitialized store", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(false)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			ldcomponents.ExternalUpdatesOnly(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Defaults, fdv1.DataAvailability())
	})

	t.Run("normal mode with no store and data source not initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()

		fdv1, err := NewFDv1(false, nil, mocks.DataSourceThatNeverInitializes(), clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Defaults, fdv1.DataAvailability())
	})

	t.Run("normal mode with no store and data source initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()

		fdv1, err := NewFDv1(false, nil, mocks.DataSourceThatIsAlwaysInitialized(), clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Refreshed, fdv1.DataAvailability())
	})

	t.Run("normal mode with initialized store and data source not initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			mocks.DataSourceThatNeverInitializes(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Cached, fdv1.DataAvailability())
	})

	t.Run("normal mode with initialized store and data source initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			mocks.DataSourceThatIsAlwaysInitialized(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Refreshed, fdv1.DataAvailability())
	})

	t.Run("normal mode with uninitialized store and data source not initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(false)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			mocks.DataSourceThatNeverInitializes(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Defaults, fdv1.DataAvailability())
	})

	t.Run("normal mode with uninitialized store and data source initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(false)

		fdv1, err := NewFDv1(false,
			mocks.SingleComponentConfigurer[subsystems.DataStore]{Instance: store},
			mocks.DataSourceThatIsAlwaysInitialized(),
			clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Refreshed, fdv1.DataAvailability())
	})
}

func TestFDv1TargetAvailability(t *testing.T) {
	t.Run("offline mode", func(t *testing.T) {
		clientContext := makeTestClientContext()
		clientContext.Offline = true

		fdv1, err := NewFDv1(true, nil, nil, clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Defaults, fdv1.TargetAvailability())
	})

	t.Run("LDD mode (daemon mode)", func(t *testing.T) {
		clientContext := makeTestClientContext()

		fdv1, err := NewFDv1(false, nil, ldcomponents.ExternalUpdatesOnly(), clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Cached, fdv1.TargetAvailability())
	})

	t.Run("normal mode", func(t *testing.T) {
		clientContext := makeTestClientContext()

		fdv1, err := NewFDv1(false, nil, mocks.DataSourceThatIsAlwaysInitialized(), clientContext)
		require.NoError(t, err)
		defer fdv1.Stop()

		assert.Equal(t, Refreshed, fdv1.TargetAvailability())
	})
}

// mockDataSystemConfigBuilder creates a mock DataSystemConfiguration builder for testing
type mockDataSystemConfigBuilder struct {
	store          subsystems.DataStore
	storeMode      subsystems.DataStoreMode
	initializers   []subsystems.DataInitializer
	hasSyncBuilder bool
}

func (m mockDataSystemConfigBuilder) Build(clientContext subsystems.ClientContext) (subsystems.DataSystemConfiguration, error) {
	config := subsystems.DataSystemConfiguration{
		Store:        m.store,
		StoreMode:    m.storeMode,
		Initializers: m.initializers,
	}

	if m.hasSyncBuilder {
		config.Synchronizers.PrimaryBuilder = func() (subsystems.DataSynchronizer, error) {
			return &mockSynchronizer{}, nil
		}
	}

	return config, nil
}

// mockSynchronizer is a minimal synchronizer implementation for testing
type mockSynchronizer struct{}

func (m *mockSynchronizer) Name() string { return "mock" }
func (m *mockSynchronizer) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, error) {
	return nil, nil
}

func (m *mockSynchronizer) Sync(store subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	ch := make(chan subsystems.DataSynchronizerResult)
	close(ch)
	return ch
}
func (m *mockSynchronizer) Close() error { return nil }

func TestFDv2DataAvailability(t *testing.T) {
	t.Run("no data sources and no store provided in data system config", func(t *testing.T) {
		clientContext := makeTestClientContext()

		configBuilder := mockDataSystemConfigBuilder{
			store:          nil,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Defaults, fdv2.DataAvailability())
	})

	t.Run("no data sources but store provided in read-only mode that is initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		configBuilder := mockDataSystemConfigBuilder{
			store:          store,
			storeMode:      subsystems.DataStoreModeRead,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Cached, fdv2.DataAvailability())
	})

	t.Run("no data sources but store provided in read-only mode that is not initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(false)

		configBuilder := mockDataSystemConfigBuilder{
			store:          store,
			storeMode:      subsystems.DataStoreModeRead,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Defaults, fdv2.DataAvailability())
	})

	t.Run("data sources configured without a store", func(t *testing.T) {
		clientContext := makeTestClientContext()

		configBuilder := mockDataSystemConfigBuilder{
			store:          nil,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: true,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)

		// Since we have a synchronizer but no data yet, we should have Defaults
		// (store.Selector() is not defined yet)
		assert.Equal(t, Defaults, fdv2.DataAvailability())
		fdv2.Stop()
	})

	t.Run("data sources configured with store that is not initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(false)

		configBuilder := mockDataSystemConfigBuilder{
			store:          store,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: true,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Defaults, fdv2.DataAvailability())
	})

	t.Run("data sources configured with store that is initialized", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		configBuilder := mockDataSystemConfigBuilder{
			store:          store,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: true,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		// Store is initialized but no data received from synchronizer yet
		// So we have cached data
		assert.Equal(t, Cached, fdv2.DataAvailability())
	})
}

func TestFDv2TargetAvailability(t *testing.T) {
	t.Run("disabled mode", func(t *testing.T) {
		clientContext := makeTestClientContext()

		configBuilder := mockDataSystemConfigBuilder{
			store:          nil,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(true, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Defaults, fdv2.TargetAvailability())
	})

	t.Run("data sources configured", func(t *testing.T) {
		clientContext := makeTestClientContext()

		configBuilder := mockDataSystemConfigBuilder{
			store:          nil,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: true,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Refreshed, fdv2.TargetAvailability())
	})

	t.Run("daemon mode (no data sources and no store)", func(t *testing.T) {
		clientContext := makeTestClientContext()

		configBuilder := mockDataSystemConfigBuilder{
			store:          nil,
			storeMode:      subsystems.DataStoreModeReadWrite,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		assert.Equal(t, Defaults, fdv2.TargetAvailability())
	})

	t.Run("store provided without data sources (not daemon mode)", func(t *testing.T) {
		clientContext := makeTestClientContext()
		store := makeTestStore(true)

		configBuilder := mockDataSystemConfigBuilder{
			store:          store,
			storeMode:      subsystems.DataStoreModeRead,
			initializers:   nil,
			hasSyncBuilder: false,
		}

		fdv2, err := NewFDv2(false, configBuilder, clientContext, nil)
		require.NoError(t, err)
		defer fdv2.Stop()

		// Has a store but no data sources, so not daemon mode
		assert.Equal(t, Cached, fdv2.TargetAvailability())
	})
}
