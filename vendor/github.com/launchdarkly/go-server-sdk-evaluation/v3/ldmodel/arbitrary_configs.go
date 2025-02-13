package ldmodel

type ArbitraryConfigType string

const (
	KeyValuesType ArbitraryConfigType = "key_value"
	ArrayType     ArbitraryConfigType = "array"
)

// ArbitraryConfigs describes an individual arbitrary configuration.
//
// The fields of this struct are exported for use by LaunchDarkly internal components.
type ArbitraryConfigs struct {
	// Key is the unique string key of the arbitrary configuration.
	Key string

	// DataType represents the data type of the configuration.
	DataType ArbitraryConfigType

	// Values contains any associated configuration values.
	Values any

	// Version is an integer that is incremented by LaunchDarkly every time the configuration is
	// changed.
	Version int
}
