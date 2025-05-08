package subsystems

// Payload represents a payload delivered in a streaming response.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type Payload struct {
	// The id here doesn't seem to match the state that is included in the
	// Payload transferred object.

	// It would be nice if we had the same value available in both so we could
	// use that as the key consistently throughout the the process.
	ID     string     `json:"id"`
	Target int        `json:"target"`
	Code   IntentCode `json:"intentCode"`
	Reason string     `json:"reason"`
}

// PollingPayload represents a payload that is delivered in a polling response.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type PollingPayload struct {
	// Note: the first event in a PollingPayload should be a Payload.
	Events []RawEvent `json:"events"`
}
