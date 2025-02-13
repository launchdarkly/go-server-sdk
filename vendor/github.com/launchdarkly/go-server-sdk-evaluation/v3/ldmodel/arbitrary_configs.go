package ldmodel

import "github.com/launchdarkly/go-sdk-common/v3/ldvalue"

type ArbitraryConfigType string

const (
	KeyValuesType ArbitraryConfigType = "key_value"
	ArrayType     ArbitraryConfigType = "array"
	UnknownType   ArbitraryConfigType = ""
)

// ArbitraryConfigs describes an individual arbitrary configuration.
//
// The fields of this struct are exported for use by LaunchDarkly internal components.
type ArbitraryConfigs struct {
	// Key is the unique string key of the arbitrary configuration.
	Key string `json:"key"`

	// DataType represents the data type of the configuration.
	DataType ArbitraryConfigType `json:"dataType"`

	// Values contains any associated configuration values.
	Values ldvalue.Value `json:"values"`

	// Version is an integer that is incremented by LaunchDarkly every time the configuration is
	// changed.
	Version int `json:"version"`
}

// StringToDataType converts a string representation to an ArbitraryConfigType.
// Returns KeyValuesType if the string matches "key_value", ArrayType if it matches "array",
// and an empty ArbitraryConfigType otherwise.
func StringToDataType(s string) ArbitraryConfigType {
	switch s {
	case string(KeyValuesType):
		return KeyValuesType
	case string(ArrayType):
		return ArrayType
	default:
		return UnknownType
	}
}
