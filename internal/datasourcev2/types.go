package datasourcev2

import (
	"encoding/json"

	es "github.com/launchdarkly/eventsource"
)

type pollingPayload struct {
	Events []event `json:"events"`
}

type event struct {
	Name      string          `json:"name"`
	EventData json.RawMessage `json:"data"`
}

// Begin es.Event interface implementation

// Id returns the id of the event.
func (e event) Id() string { //nolint:stylecheck // The interface requires this method.
	return ""
}

// Event returns the name of the event.
func (e event) Event() string {
	return e.Name
}

// Data returns the raw data of the event.
func (e event) Data() string {
	return string(e.EventData)
}

// En es.Event interface implementation

type changeSet struct {
	intent *ServerIntent
	events []es.Event
}

type ServerIntent struct {
	Payloads []Payload `json:"payloads"`
}

type Payload struct {
	// The id here doesn't seem to match the state that is included in the
	// payload transferred object.

	// It would be nice if we had the same value available in both so we could
	// use that as the key consistently throughout the the process.
	ID     string `json:"id"`
	Target int    `json:"target"`
	Code   string `json:"intentCode"`
	Reason string `json:"reason"`
}

// This is the general shape of a put-object event. The delete-object is the same, with the object field being nil.
// type baseObject struct {
// 	Version int             `json:"version"`
// 	Kind    string          `json:"kind"`
// 	Key     string          `json:"key"`
// 	Object  json.RawMessage `json:"object"`
// }

// type payloadTransferred struct {
// 	State   string `json:"state"`
// 	Version int    `json:"version"`
// }

// TODO: Todd doesn't have this in his spec. What are we going to do here?
//
//nolint:godox
type errorEvent struct {
	PayloadID string `json:"payloadId"`
	Reason    string `json:"reason"`
}

// type heartBeat struct{}

type goodbye struct {
	Reason      string `json:"reason"`
	Silent      bool   `json:"silent"`
	Catastrophe bool   `json:"catastrophe"`
	//nolint:godox
	// TODO: Might later include some advice or backoff information
}
