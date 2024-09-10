package datasystem

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestStore_New(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	assert.NoError(t, store.Close())
}

func TestStore_NoPersistence_NewStore_DataStatus(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.Equal(t, store.DataStatus(), Defaults)
}

func TestStore_NoPersistence_NewStore_IsInitialized(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.False(t, store.IsInitialized())
}

func TestStore_NoPersistence_MemoryStoreInitialized_DataStatus(t *testing.T) {
	tests := []struct {
		name      string
		refreshed bool
		expected  DataStatus
	}{
		{"fresh data", true, Refreshed},
		{"cached data", false, Cached},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logCapture := ldlogtest.NewMockLog()
			store := NewStore(logCapture.Loggers)
			defer store.Close()
			store.Init([]ldstoretypes.Collection{})
			assert.Equal(t, store.DataStatus(), Cached)
			assert.True(t, store.IsInitialized())
			store.SwapToMemory(tt.refreshed)
			assert.Equal(t, store.DataStatus(), tt.expected)
		})
	}
}

func TestStore_NoPersistence_Commit_NoCrashesCaused(t *testing.T) {
	logCapture := ldlogtest.NewMockLog()
	store := NewStore(logCapture.Loggers)
	defer store.Close()
	assert.NoError(t, store.Commit())
}

func TestStore_GetActive(t *testing.T) {
	t.Run("memory store is active if no persistent store configured", func(t *testing.T) {
		logCapture := ldlogtest.NewMockLog()
		store := NewStore(logCapture.Loggers)
		defer store.Close()
		foo, err := store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, foo, ldstoretypes.ItemDescriptor{}.NotFound())

		assert.True(t, store.Init([]ldstoretypes.Collection{
			{Kind: ldstoreimpl.Features(), Items: []ldstoretypes.KeyedItemDescriptor{
				{Key: "foo", Item: ldstoretypes.ItemDescriptor{Version: 1}},
			}},
		}))

		foo, err = store.Get(ldstoreimpl.Features(), "foo")
		assert.NoError(t, err)
		assert.Equal(t, 1, foo.Version)
	})
}
