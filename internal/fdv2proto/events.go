package fdv2proto

import (
	"fmt"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
)

type IntentCode string

const (
	IntentTransferFull    = IntentCode("xfer-full")
	IntentTransferChanges = IntentCode("xfer-changes")
)

type Event interface {
	Name() EventName
}

type EventName string

const (
	EventPutObject          = EventName("put-object")
	EventDeleteObject       = EventName("delete-object")
	EventServerIntent       = EventName("server-intent")
	EventPayloadTransferred = EventName("payload-transferred")
	EventHeartbeat          = EventName("heart-beat")
	EventGoodbye            = EventName("goodbye")
	EventError              = EventName("error")
)

type ObjectKind string

const (
	FlagKind    = ObjectKind("flag")
	SegmentKind = ObjectKind("segment")
)

func (o ObjectKind) ToFDV1() (datakinds.DataKindInternal, error) {
	switch o {
	case FlagKind:
		return datakinds.Features, nil
	case SegmentKind:
		return datakinds.Segments, nil
	default:
		return nil, fmt.Errorf("no FDv1 equivalent for object kind (%s)", string(o))
	}
}

type ServerIntent struct {
	Payloads []Payload `json:"payloads"`
}

func (ServerIntent) Name() EventName {
	return EventServerIntent
}

type PayloadTransferred struct {
	State   string `json:"state"`
	Version int    `json:"version"`
}

func (p PayloadTransferred) Name() EventName {
	return EventPayloadTransferred
}

type DeleteObject struct {
	Version int        `json:"version"`
	Kind    ObjectKind `json:"kind"`
	Key     string     `json:"key"`
}

func (d DeleteObject) Name() EventName {
	return EventDeleteObject
}

type PutObject struct {
	Version int        `json:"version"`
	Kind    ObjectKind `json:"kind"`
	Key     string     `json:"key"`
	Object  any        `json:"object"`
}

func (p PutObject) Name() EventName {
	return EventPutObject
}

type Error struct {
	PayloadID string `json:"payloadId"`
	Reason    string `json:"reason"`
}

func (e Error) Name() EventName {
	return EventError
}

type Goodbye struct {
	Reason      string `json:"reason"`
	Silent      bool   `json:"silent"`
	Catastrophe bool   `json:"catastrophe"`
}

func (g Goodbye) Name() EventName {
	return EventGoodbye
}
