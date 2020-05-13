// Package utils contains support code that most users of the SDK will not need to access
// directly. However, they may be useful for anyone developing custom integrations.
package utils

import (
	"encoding/json"
	"fmt"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// UnmarshalItem attempts to unmarshal an entity that has been stored as JSON in a
// DataStore. The kind parameter indicates what type of entity is expected.
func UnmarshalItem(kind interfaces.VersionedDataKind, raw []byte) (interfaces.VersionedData, error) {
	data := kind.GetDefaultItem()
	if jsonErr := json.Unmarshal(raw, &data); jsonErr != nil {
		return nil, jsonErr
	}
	if item, ok := data.(interfaces.VersionedData); ok {
		return item, nil
	}
	return nil, fmt.Errorf("unexpected data type from JSON unmarshal: %T", data)
}
