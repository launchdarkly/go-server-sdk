package datakinds

import (
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
)

// DataKindInternal is implemented along with DataKind to provide more efficient jsonstream-based
// deserialization for our built-in data kinds.
type DataKindInternal interface {
	ldstoretypes.DataKind
	DeserializeFromJSONReader(reader *jreader.Reader) (ldstoretypes.ItemDescriptor, error)
}
