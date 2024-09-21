package fdv2proto

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

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

type DeleteObject struct {
	Version int
	Kind    ldstoretypes.DataKind
	Key     string
}

func (d DeleteObject) Name() EventName {
	return EventDeleteObject
}

type PutObject struct {
	Version int
	Kind    ldstoretypes.DataKind
	Key     string
	Object  ldstoretypes.ItemDescriptor
}

func (p PutObject) Name() EventName {
	return EventPutObject
}
