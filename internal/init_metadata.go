package internal

import (
	"net/http"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

// InitMetadata contains initialization metadata that is parsed from server response headers.
type InitMetadata struct {
	environmentID ldvalue.OptionalString
}

// GetEnvironmentID gets the EnvironmentID as an optional string.
func (m InitMetadata) GetEnvironmentID() ldvalue.OptionalString {
	return m.environmentID
}

// NewInitMetadataFromHeaders builds a new InitMetadata instance from the provided headers.
//
// EnvironmentID will be parsed from the "X-Ld-Envid" header, if it is present.
func NewInitMetadataFromHeaders(headers http.Header) InitMetadata {
	if headers == nil {
		return InitMetadata{}
	}
	environmentID := headers.Get("X-Ld-Envid")
	if len(environmentID) > 0 {
		return InitMetadata{
			environmentID: ldvalue.NewOptionalString(environmentID),
		}
	} else {
		return InitMetadata{
			environmentID: ldvalue.OptionalString{},
		}
	}
}
