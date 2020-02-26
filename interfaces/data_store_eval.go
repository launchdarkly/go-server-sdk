package interfaces

import (
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

// Implementation of ldeval.DataProvider
type dataStoreEvaluatorDataProvider struct {
	store DataStore
}

// NewDataStoreEvaluatorDataProvider provides an adapter for using a DataStore with the Evaluator type
// in go-server-sdk-evaluation.
//
// Normal use of the SDK does not require this type. It is provided for use by other LaunchDarkly
// components that use DataStore and Evaluator separately from the SDK.
func NewDataStoreEvaluatorDataProvider(store DataStore) ldeval.DataProvider {
	return dataStoreEvaluatorDataProvider{store}
}

func (d dataStoreEvaluatorDataProvider) GetFeatureFlag(key string) (ldmodel.FeatureFlag, bool) {
	data, err := d.store.Get(dataKindFeatures, key)
	if data != nil && err == nil {
		if flag, ok := data.(*ldmodel.FeatureFlag); ok {
			return *flag, true
		}
	}
	return ldmodel.FeatureFlag{}, false
}

func (d dataStoreEvaluatorDataProvider) GetSegment(key string) (ldmodel.Segment, bool) {
	data, err := d.store.Get(dataKindSegments, key)
	if data != nil && err == nil {
		if segment, ok := data.(*ldmodel.Segment); ok {
			return *segment, true
		}
	}
	return ldmodel.Segment{}, false
}
