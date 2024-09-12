package ldservicesv2

import (
	"encoding/json"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasourcev2"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

type ProtocolEvents []httphelpers.SSEEvent

func (p ProtocolEvents) Enqueue(control httphelpers.SSEStreamControl) {
	for _, msg := range p {
		control.Enqueue(msg)
	}
}

type protoState string

const (
	start       = protoState("start")
	intentSent  = protoState("intent-sent")
	transferred = protoState("transferred")
)

type BaseObject struct {
	Version int             `json:"version"`
	Kind    string          `json:"kind"`
	Key     string          `json:"key"`
	Object  json.RawMessage `json:"object"`
}

type event struct {
	name string
	data BaseObject
}

type payloadTransferred struct {
	State   string `json:"state"`
	Version int    `json:"version"`
}

type StreamingProtocol struct {
	events []httphelpers.SSEEvent
}

func NewStreamingProtocol() *StreamingProtocol {
	return &StreamingProtocol{}
}

func (f *StreamingProtocol) WithIntent(intent datasourcev2.ServerIntent) *StreamingProtocol {
	return f.pushEvent("server-intent", intent)
}

func (f *StreamingProtocol) WithPutObject(object BaseObject) *StreamingProtocol {
	return f.pushEvent("put-object", object)
}

func (f *StreamingProtocol) WithTransferred() *StreamingProtocol {
	return f.pushEvent("payload-transferred", payloadTransferred{State: "[p:17YNC7XBH88Y6RDJJ48EKPCJS7:53]", Version: 1})
}

func (f *StreamingProtocol) WithPutObjects(objects []BaseObject) *StreamingProtocol {
	for _, object := range objects {
		f.WithPutObject(object)
	}
	return f
}

func (f *StreamingProtocol) pushEvent(event string, data any) *StreamingProtocol {
	marshalled, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	f.events = append(f.events, httphelpers.SSEEvent{Event: event, Data: string(marshalled)})
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
