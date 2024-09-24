package fdv2proto

import (
	"encoding/json"
)

type RawEvent struct {
	Name      EventName       `json:"name"`
	EventData json.RawMessage `json:"data"`
}

// Begin es.Event interface implementation

// Id returns the id of the event.
func (e RawEvent) Id() string { //nolint:stylecheck // The interface requires this method.
	return ""
}

// Event returns the name of the event.
func (e RawEvent) Event() string {
	return string(e.Name)
}

// Data returns the raw data of the event.
func (e RawEvent) Data() string {
	return string(e.EventData)
}
