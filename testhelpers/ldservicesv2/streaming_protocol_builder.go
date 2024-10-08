package ldservicesv2

import (
	"encoding/json"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

// ProtocolEvents represents a list of SSE-formatted events.
type ProtocolEvents []httphelpers.SSEEvent

// Enqueue adds all the events to an SSEStreamController.
func (p ProtocolEvents) Enqueue(control httphelpers.SSEStreamControl) {
	for _, msg := range p {
		control.Enqueue(msg)
	}
}

// StreamingProtocol is a builder for creating a sequence of events that can be sent as an SSE stream.
type StreamingProtocol struct {
	events ProtocolEvents
}

// NewStreamingProtocol creates a new StreamingProtocol instance.
func NewStreamingProtocol() *StreamingProtocol {
	return &StreamingProtocol{}
}

// WithIntent adds a ServerIntent event to the protocol.
func (f *StreamingProtocol) WithIntent(intent fdv2proto.ServerIntent) *StreamingProtocol {
	return f.pushEvent(intent)
}

// WithPutObject adds a PutObject event to the protocol.
func (f *StreamingProtocol) WithPutObject(object fdv2proto.PutObject) *StreamingProtocol {
	return f.pushEvent(object)
}

// WithTransferred adds a PayloadTransferred event to the protocol with a given version. The state is a a placeholder
// string.
func (f *StreamingProtocol) WithTransferred(version int) *StreamingProtocol {
	return f.pushEvent(fdv2proto.PayloadTransferred{State: "[p:17YNC7XBH88Y6RDJJ48EKPCJS7:53]", Version: version})
}

// WithPutObjects adds multiple PutObject events to the protocol.
func (f *StreamingProtocol) WithPutObjects(objects []fdv2proto.PutObject) *StreamingProtocol {
	for _, object := range objects {
		f.WithPutObject(object)
	}
	return f
}

func (f *StreamingProtocol) pushEvent(data fdv2proto.Event) *StreamingProtocol {
	marshalled, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	f.events = append(f.events, httphelpers.SSEEvent{Event: string(data.Name()), Data: string(marshalled)})
	return f
}

// HasNext returns true if there are more events in the protocol.
func (f *StreamingProtocol) HasNext() bool {
	return len(f.events) != 0
}

// Next returns the next event in the protocol, popping the event from protocol's internal queue.
func (f *StreamingProtocol) Next() httphelpers.SSEEvent {
	if !f.HasNext() {
		panic("protocol has no events")
	}
	event := f.events[0]
	f.events = f.events[1:]
	return event
}

// Enqueue adds all the events to an SSEStreamController.
func (f *StreamingProtocol) Enqueue(control httphelpers.SSEStreamControl) {
	f.events.Enqueue(control)
	f.events = nil
}
