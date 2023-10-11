package ldstoreimpl

import (
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// This file contains the public API for accessing things in internal/datakinds. We need to export
// these things in order to support development of custom database integrations and internal LD
// components, but we don't want to expose the underlying global variables.

// AllKinds returns a list of supported StoreDataKinds. Among other things, this list might
// be used by data stores to know what data (namespaces) to expect.
func AllKinds() []ldstoretypes.DataKind {
	return datakinds.AllDataKinds()
}

// Features returns the StoreDataKind instance corresponding to feature flag data.
func Features() ldstoretypes.DataKind {
	return datakinds.Features
}

// Segments returns the StoreDataKind instance corresponding to user segment data.
func Segments() ldstoretypes.DataKind {
	return datakinds.Segments
}
