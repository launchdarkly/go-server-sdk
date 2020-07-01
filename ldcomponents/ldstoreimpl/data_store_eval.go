package ldstoreimpl

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
)

// This file contains the public API for creating the adapter that bridges Evaluator to DataStore. The
// actual implementation is in internal/datastore, but we expose it because it is helpful when we
// evaluate flags outside of the SDK in ld-relay.

// NewDataStoreEvaluatorDataProvider provides an adapter for using a DataStore with the Evaluator type
// in go-server-sdk-evaluation.
//
// Normal use of the SDK does not require this type. It is provided for use by other LaunchDarkly
// components that use DataStore and Evaluator separately from the SDK.
func NewDataStoreEvaluatorDataProvider(store interfaces.DataStore, loggers ldlog.Loggers) ldeval.DataProvider {
	return datastore.NewDataStoreEvaluatorDataProviderImpl(store, loggers)
}
