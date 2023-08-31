package ldstoreimpl

import (
	"testing"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

func TestDataStoreEvaluatorDataProvider(t *testing.T) {
	// The underlying implementation type is tested in the internal/datastore package, so this test
	// just verifies that we are in fact constructing that type.
	loggers := ldlog.NewDisabledLoggers()
	store := datastore.NewInMemoryDataStore(loggers)
	provider := NewDataStoreEvaluatorDataProvider(store, loggers)
	expected := datastore.NewDataStoreEvaluatorDataProviderImpl(store, loggers)
	assert.Equal(t, expected, provider)
}
