package internal

import "github.com/launchdarkly/go-sdk-common/v3/ldvalue"

// EnvironmentIDProvider describes the interface for an object that can provide an EnvironmentID.
type EnvironmentIDProvider interface {
	// GetEnvironmentID gets the provided EnvironmentID as an optional string.
	GetEnvironmentID() ldvalue.OptionalString
}
