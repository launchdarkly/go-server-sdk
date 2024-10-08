package fdv2proto

import (
	"encoding/json"
)

// RawEvent is a partially deserialized event that allows the the event name to be extracted before
// the rest of the event is deserialized.
type RawEvent struct {
	Name EventName       `json:"name"`
	Data json.RawMessage `json:"data"`
}
