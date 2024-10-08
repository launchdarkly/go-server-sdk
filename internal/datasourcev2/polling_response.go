package datasourcev2

import "github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

// PollingResponse represents the result of a polling request.
type PollingResponse struct {
	events   []fdv2proto.Event
	cached   bool
	intent   fdv2proto.IntentCode
	selector *fdv2proto.Selector
}

// NewCachedPollingResponse indicates that the response has not changed.
func NewCachedPollingResponse() *PollingResponse {
	return &PollingResponse{
		cached: true,
	}
}

// NewPollingResponse indicates that data was received.
func NewPollingResponse(intent fdv2proto.IntentCode, events []fdv2proto.Event,
	selector *fdv2proto.Selector) *PollingResponse {
	return &PollingResponse{
		events:   events,
		intent:   intent,
		selector: selector,
	}
}

// Events returns the events in the response.
func (p *PollingResponse) Events() []fdv2proto.Event {
	return p.events
}

// Cached returns true if the response was cached, meaning data has not changed.
func (p *PollingResponse) Cached() bool {
	return p.cached
}

// Intent returns the server intent code of the response.
func (p *PollingResponse) Intent() fdv2proto.IntentCode {
	return p.intent
}

// Selector returns the Selector of the response.
func (p *PollingResponse) Selector() *fdv2proto.Selector {
	return p.selector
}
