package subsystems

import (
	"encoding/json"
)

// RawEvent is a partially deserialized event that allows the event name to be extracted before
// the rest of the event is deserialized.
//
// This type is not stable, and not subject to any backwards
// compatibility guarantees or semantic versioning. It is not suitable for production usage.
//
// Do not use it.
// You have been warned.
type RawEvent struct {
	Name EventName       `json:"event"`
	Data json.RawMessage `json:"data"`
}
