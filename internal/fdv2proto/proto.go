package fdv2proto

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

type IntentCode string

const (
	IntentTransferFull    = IntentCode("xfer-full")
	IntentTransferChanges = IntentCode("xfer-changes")
)

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

type Selector struct {
	state   string
	version int
	set     bool
}

func NoSelector() Selector {
	return Selector{set: false}
}

func NewSelector(state string, version int) Selector {
	return Selector{state: state, version: version, set: true}
}

func (s Selector) IsSet() bool {
	return s.set
}

func (s Selector) State() string {
	return s.state
}

func (s Selector) Version() int {
	return s.version
}

func (s Selector) Get() (string, int, bool) {
	return s.state, s.version, s.set
}

type Event interface {
	Name() EventName
}

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
