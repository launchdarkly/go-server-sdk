package datastore

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type dataStoreEvaluatorDataProviderImpl struct {
	store   subsystems.DataStore
	loggers ldlog.Loggers
}

// NewDataStoreEvaluatorDataProviderImpl creates the internal implementation of the adapter that connects
// the Evaluator (from go-server-sdk-evaluation) with the data store.
func NewDataStoreEvaluatorDataProviderImpl(store subsystems.DataStore, loggers ldlog.Loggers) ldeval.DataProvider {
	return dataStoreEvaluatorDataProviderImpl{store, loggers}
}

func (d dataStoreEvaluatorDataProviderImpl) GetFeatureFlag(key string) *ldmodel.FeatureFlag {
	item, err := d.store.Get(datakinds.Features, key)
	if err == nil && item.Item != nil {
		data := item.Item
		if flag, ok := data.(*ldmodel.FeatureFlag); ok {
			return flag
		}
		d.loggers.Errorf("unexpected data type (%T) found in store for feature key: %s. Returning default value", data, key)
	}
	return nil
}

func (d dataStoreEvaluatorDataProviderImpl) GetSegment(key string) *ldmodel.Segment {
	item, err := d.store.Get(datakinds.Segments, key)
	if err == nil && item.Item != nil {
		data := item.Item
		if segment, ok := data.(*ldmodel.Segment); ok {
			return segment
		}
		d.loggers.Errorf("unexpected data type (%T) found in store for segment key: %s. Returning default value", data, key)
	}
	return nil
}
