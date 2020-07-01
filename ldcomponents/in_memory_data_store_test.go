package ldcomponents

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryDataStoreFactory(t *testing.T) {
	factory := InMemoryDataStore()
	store, err := factory.CreateDataStore(basicClientContext(), nil)
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.IsType(t, datastore.NewInMemoryDataStore(sharedtest.NewTestLoggers()), store)
}
