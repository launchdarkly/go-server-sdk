package fdv2proto

import "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

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

// DeleteObject specifies the deletion of a particular object.
type DeleteObject struct {
	Version int
	Kind    ldstoretypes.DataKind
	Key     string
}

//nolint:revive // Event method.
func (d DeleteObject) Name() EventName {
	return EventDeleteObject
}

// PutObject specifies the addition of a particular object with upsert semantics.
type PutObject struct {
	Version int
	Kind    ldstoretypes.DataKind
	Key     string
	Object  ldstoretypes.ItemDescriptor
}

//nolint:revive // Event method.
func (p PutObject) Name() EventName {
	return EventPutObject
}
