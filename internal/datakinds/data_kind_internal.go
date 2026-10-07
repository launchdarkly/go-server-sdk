package datakinds

import (
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
)

// DeserializeOptions controls how the built-in data kinds deserialize flags and segments. The zero
// value is the default behavior. The fields are unexported, so that code outside the SDK cannot make
// a value for exported methods such as subsystems.ChangeSetBuilder.WithDeserializeOptions.
type DeserializeOptions struct {
	releaseClauseValues bool
}

// NewDeserializeOptions returns options for deserialization. If releaseClauseValues is true,
// deserialization releases the Values list of each "in" clause that preprocessing put into a lookup
// set. For details, see ldmodel.ReleaseClauseValues. This is the value of Config.ReleaseClauseValues.
func NewDeserializeOptions(releaseClauseValues bool) DeserializeOptions {
	return DeserializeOptions{releaseClauseValues: releaseClauseValues}
}

// ReleaseClauseValues returns true if deserialization releases redundant clause value lists.
func (o DeserializeOptions) ReleaseClauseValues() bool {
	return o.releaseClauseValues
}

// DataKindInternal is implemented along with DataKind to provide more efficient jsonstream-based
// deserialization for our built-in data kinds.
type DataKindInternal interface {
	ldstoretypes.DataKind
	DeserializeFromJSONReader(reader *jreader.Reader) (ldstoretypes.ItemDescriptor, error)
	// DeserializeWithOptions is the same as Deserialize, but uses the options.
	DeserializeWithOptions(data []byte, opts DeserializeOptions) (ldstoretypes.ItemDescriptor, error)
	// DeserializeFromJSONReaderWithOptions is the same as DeserializeFromJSONReader, but uses the options.
	DeserializeFromJSONReaderWithOptions(reader *jreader.Reader, opts DeserializeOptions) (
		ldstoretypes.ItemDescriptor, error)
}

// Deserialize uses the options if kind is a built-in data kind. Otherwise it calls kind.Deserialize,
// and does not use the options.
func Deserialize(kind ldstoretypes.DataKind, data []byte, opts DeserializeOptions) (
	ldstoretypes.ItemDescriptor, error) {
	if internalKind, ok := kind.(DataKindInternal); ok {
		return internalKind.DeserializeWithOptions(data, opts)
	}
	return kind.Deserialize(data)
}
