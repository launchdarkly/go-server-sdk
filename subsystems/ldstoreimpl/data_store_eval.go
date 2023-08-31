package ldstoreimpl

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk/v6/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"
)

// This file contains the public API for creating the adapter that bridges Evaluator to DataStore. The
// actual implementation is in internal/datastore, but we expose it because it is helpful when we
// evaluate flags outside of the SDK in ld-relay.

// NewDataStoreEvaluatorDataProvider provides an adapter for using a DataStore with the Evaluator type
// in go-server-sdk-evaluation.
//
// Normal use of the SDK does not require this type. It is provided for use by other LaunchDarkly
// components that use DataStore and Evaluator separately from the SDK.
func NewDataStoreEvaluatorDataProvider(store subsystems.DataStore, loggers ldlog.Loggers) ldeval.DataProvider {
	return datastore.NewDataStoreEvaluatorDataProviderImpl(store, loggers)
}
