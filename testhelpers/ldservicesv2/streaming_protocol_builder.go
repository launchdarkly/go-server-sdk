package ldservicesv2

import (
	"encoding/json"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

type ProtocolEvents []httphelpers.SSEEvent

func (p ProtocolEvents) Enqueue(control httphelpers.SSEStreamControl) {
	for _, msg := range p {
		control.Enqueue(msg)
	}
}

type StreamingProtocol struct {
	events []httphelpers.SSEEvent
}

func NewStreamingProtocol() *StreamingProtocol {
	return &StreamingProtocol{}
}

func (f *StreamingProtocol) WithIntent(intent fdv2proto.ServerIntent) *StreamingProtocol {
	return f.pushEvent(intent)
}

func (f *StreamingProtocol) WithPutObject(object fdv2proto.PutObject) *StreamingProtocol {
	return f.pushEvent(object)
}

func (f *StreamingProtocol) WithTransferred() *StreamingProtocol {
	return f.pushEvent(fdv2proto.PayloadTransferred{State: "[p:17YNC7XBH88Y6RDJJ48EKPCJS7:53]", Version: 1})
}

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

func (f *StreamingProtocol) HasNext() bool {
	return len(f.events) != 0
}

func (f *StreamingProtocol) Next() httphelpers.SSEEvent {
	if !f.HasNext() {
		panic("protocol has no events")
	}
	event := f.events[0]
	f.events = f.events[1:]
	return event
}

func (f *StreamingProtocol) Enqueue(control httphelpers.SSEStreamControl) {
	for _, event := range f.events {
		control.Enqueue(event)
	}
	f.events = nil
}
