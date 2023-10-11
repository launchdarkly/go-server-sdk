package ldcomponents

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryDataStoreFactory(t *testing.T) {
	factory := InMemoryDataStore()
	store, err := factory.Build(basicClientContext())
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.IsType(t, datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()), store)
}
