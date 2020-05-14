package ldcomponents

import (
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryDataStoreFactory(t *testing.T) {
	factory := InMemoryDataStore()
	store, err := factory.CreateDataStore(basicClientContext())
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.IsType(t, internal.NewInMemoryDataStore(ldlog.NewDisabledLoggers()), store)
}
