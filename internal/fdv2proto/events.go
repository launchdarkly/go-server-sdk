package fdv2proto

import (
	"fmt"

	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
)

// IntentCode represents the various intents that can be sent by the server.
type IntentCode string

const (
	// IntentTransferFull means the server intends to send a full data set.
	IntentTransferFull = IntentCode("xfer-full")
	// IntentTransferChanges means the server intends to send only the necessary changes to bring
	// an existing data set up-to-date.
	IntentTransferChanges = IntentCode("xfer-changes")
)

// Event represents an event that can be sent by the server.
type Event interface {
	// Name returns the name of the event.
	Name() EventName
}

// EventName is the name of the event.
type EventName string

const (
	// EventPutObject specifies that an object should be added to the data set with upsert semantics.
	EventPutObject = EventName("put-object")

	// EventDeleteObject specifies that an object should be removed from the data set.
	EventDeleteObject = EventName("delete-object")

	// EventServerIntent specifies the server's intent.
	EventServerIntent = EventName("server-intent")

	// EventPayloadTransferred specifies that that all data required to bring the existing data set to
	// a new version has been transferred.
	EventPayloadTransferred = EventName("payload-transferred")

	// EventHeartbeat keeps the connection alive.
	EventHeartbeat = EventName("heart-beat")

	// EventGoodbye specifies that the server is about to close the connection.
	EventGoodbye = EventName("goodbye")

	// EventError specifies that an error occurred while serving the connection.
	EventError = EventName("error")
)

// ObjectKind represents the kind of object.
type ObjectKind string

const (
	// FlagKind is a flag.
	FlagKind = ObjectKind("flag")
	// SegmentKind is a segment.
	SegmentKind = ObjectKind("segment")
)

// ToFDV1 converts the object kind to an FDv1 data kind.
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

// ServerIntent represents the server's intent.
type ServerIntent struct {
	// Payloads is a list of payloads, defined to be at least length 1.
	Payloads []Payload `json:"payloads"`
}

//nolint:revive // Event method.
func (ServerIntent) Name() EventName {
	return EventServerIntent
}

// PayloadTransferred represents the fact that all payload objects have been sent.
type PayloadTransferred struct {
	State   string `json:"state"`
	Version int    `json:"version"`
}

//nolint:revive // Event method.
func (p PayloadTransferred) Name() EventName {
	return EventPayloadTransferred
}

// DeleteObject specifies the deletion of a particular object.
type DeleteObject struct {
	Version int        `json:"version"`
	Kind    ObjectKind `json:"kind"`
	Key     string     `json:"key"`
}

//nolint:revive // Event method.
func (d DeleteObject) Name() EventName {
	return EventDeleteObject
}

// PutObject specifies the addition of a particular object with upsert semantics.
type PutObject struct {
	Version int        `json:"version"`
	Kind    ObjectKind `json:"kind"`
	Key     string     `json:"key"`
	Object  any        `json:"object"`
}

//nolint:revive // Event method.
func (p PutObject) Name() EventName {
	return EventPutObject
}

// Error represents an error event.
type Error struct {
	PayloadID string `json:"payloadId"`
	Reason    string `json:"reason"`
}

//nolint:revive // Event method.
func (e Error) Name() EventName {
	return EventError
}

// Goodbye represents a goodbye event.
type Goodbye struct {
	Reason      string `json:"reason"`
	Silent      bool   `json:"silent"`
	Catastrophe bool   `json:"catastrophe"`
}

//nolint:revive // Event method.
func (g Goodbye) Name() EventName {
	return EventGoodbye
}
