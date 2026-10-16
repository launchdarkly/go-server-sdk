package datastore

import (
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// CacheExporter is a store that holds a complete copy of the current data.
type CacheExporter interface {
	// ExportAll returns every item the store holds. It reports false if the store has no data, or
	// if the data must not be written to a database.
	ExportAll() ([]st.Collection, bool)
}
