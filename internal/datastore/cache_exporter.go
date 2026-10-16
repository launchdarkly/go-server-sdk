package datastore

import (
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// CacheExporter is a store that holds a complete copy of the current data.
type CacheExporter interface {
	// ExportTo passes every item the store holds to write, and reports whether it did.
	ExportTo(write func([]st.Collection) error) (bool, error)
}
