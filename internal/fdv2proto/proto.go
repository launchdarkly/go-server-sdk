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

// DeleteData is the logical representation of the data in the "delete" event. In the JSON representation,
// there is a "path" property in the format "/flags/key" or "/segments/key", which we convert into
// Kind and Key when we parse it.
//
// Example JSON representation:
//
//	{
//	  "path": "/flags/flagkey",
//	  "version": 3
//	}
type DeleteData struct {
	Kind    ldstoretypes.DataKind
	Key     string
	Version int
}

type DeleteObject struct {
	Version int
	Kind    ldstoretypes.DataKind
	Key     string
}

func (d DeleteObject) Name() EventName {
	return EventDeleteObject
}

// PutData is the logical representation of the data in the "put" event. In the JSON representation,
// the "data" property is actually a map of maps, but the schema we use internally is a list of
// lists instead.
//
// The "path" property is normally always "/"; the LD streaming service sends this property, but
// some versions of Relay do not, so we do not require it.
//
// Example JSON representation:
//
//	{
//	  "path": "/",
//	  "data": {
//	    "flags": {
//	      "flag1": { "key": "flag1", "version": 1, ...etc. },
//	      "flag2": { "key": "flag2", "version": 1, ...etc. },
//	    },
//	    "segments": {
//	      "segment1": { "key", "segment1", "version": 1, ...etc. }
//	    }
//	  }
//	}
type PutData struct {
	Path string // we don't currently do anything with this
	Data []ldstoretypes.Collection
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

// PatchData is the logical representation of the data in the "patch" event. In the JSON representation,
// there is a "path" property in the format "/flags/key" or "/segments/key", which we convert into
// Kind and Key when we parse it. The "data" property is the JSON representation of the flag or
// segment, which we deserialize into an ItemDescriptor.
//
// Example JSON representation:
//
//	{
//	  "path": "/flags/flagkey",
//	  "data": {
//	    "key": "flagkey",
//	    "version": 2, ...etc.
//	  }
//	}
type PatchData struct {
	Kind ldstoretypes.DataKind
	Key  string
	Data ldstoretypes.ItemDescriptor
}
