package datakinds

import (
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"

	"gopkg.in/launchdarkly/go-jsonstream.v1/jreader"
)

// DataKindInternal is implemented along with DataKind to provide more efficient jsonstream-based
// deserialization for our built-in data kinds.
type DataKindInternal interface {
	ldstoretypes.DataKind
	DeserializeFromJSONReader(reader *jreader.Reader) (ldstoretypes.ItemDescriptor, error)
}
