package fdv2proto

// Payload represents a payload delivered in a streaming response.
type Payload struct {
	// The id here doesn't seem to match the state that is included in the
	// Payload transferred object.

	// It would be nice if we had the same value available in both so we could
	// use that as the key consistently throughout the the process.
	ID     string     `json:"id"`
	Target int        `json:"target"`
	Code   IntentCode `json:"code"`
	Reason string     `json:"reason"`
}

// PollingPayload represents a payload that is delivered in a polling response.
type PollingPayload struct {
	// Note: the first event in a PollingPayload should be a Payload.
	Events []RawEvent `json:"events"`
}
