package datastore

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	evaluation "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel/flag_eval"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type nextgenDatastoreEvaluatorImpl struct {
	store   subsystems.DataStore
	loggers ldlog.Loggers
}

// NewDataStoreEvaluatorDataProviderImpl creates the internal implementation of the adapter that connects
// the Evaluator (from go-server-sdk-evaluation) with the data store.
func NewNextGenDataStoreEvaluatorDataProviderImpl(store subsystems.DataStore, loggers ldlog.Loggers) ldeval.DataProvider {
	return dataStoreEvaluatorDataProviderImpl{store, loggers}
}

func (n nextgenDatastoreEvaluatorImpl) GetFeatureFlag(key string) *ldmodel.FeatureFlag {
	item, err := n.store.Get(datakinds.AudienceVariations, key)
	if err != nil {
		return nil
	}
	audienceKeys := item.Item.([]string)
	avs := []*flag_eval.AudienceVariation{}
	for _, key := range audienceKeys {
		av, err := n.store.Get(datakinds.Audiences, key)
		if err != nil {
			return nil
		}
		avs = append(avs, av.Item.(*flag_eval.AudienceVariation))
	}

	evaluation.NewFlag(key, avs)

	if err == nil && item.Item != nil {
		data := item.Item
		if flag, ok := data.(*ldmodel.FeatureFlag); ok {
			return flag
		}
		n.loggers.Errorf("unexpected data type (%T) found in store for feature key: %s. Returning default value", data, key)
	}
	return nil
}

func (n nextgenDatastoreEvaluatorImpl) GetSegment(key string) *ldmodel.Segment {
	item, err := n.store.Get(datakinds.Segments, key)
	if err == nil && item.Item != nil {
		data := item.Item
		if segment, ok := data.(*ldmodel.Segment); ok {
			return segment
		}
		n.loggers.Errorf("unexpected data type (%T) found in store for segment key: %s. Returning default value", data, key)
	}
	return nil
}
