package ldstoreimpl

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
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
