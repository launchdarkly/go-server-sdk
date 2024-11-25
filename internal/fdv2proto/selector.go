package fdv2proto

import (
	"encoding/json"
	"errors"
)

// Selector represents a particular snapshot of data.
type Selector struct {
	state   string
	version int
}

// NoSelector returns an empty Selector.
func NoSelector() Selector {
	return Selector{}
}

// IsDefined returns true if the Selector has a value.
func (s Selector) IsDefined() bool {
	return s != NoSelector()
}

//nolint:revive // Event method.
func (s Selector) Name() EventName {
	return EventPayloadTransferred
}

// NewSelector creates a new Selector from a state string and version.
func NewSelector(state string, version int) Selector {
	return Selector{state: state, version: version}
}

// State returns the state string of the Selector. This cannot be called if the Selector is nil.
func (s Selector) State() string {
	return s.state
}

// Version returns the version of the Selector. This cannot be called if the Selector is nil.
func (s Selector) Version() int {
	return s.version
}

// UnmarshalJSON unmarshals a Selector from JSON.
func (s *Selector) UnmarshalJSON(data []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if state, ok := raw["state"].(string); ok {
		s.state = state
	} else {
		return errors.New("unmarshal selector: missing state field")
	}
	if version, ok := raw["version"].(float64); ok {
		s.version = int(version)
	} else {
		return errors.New("unmarshal selector: missing version field")
	}
	return nil
}

// MarshalJSON marshals a Selector to JSON.
func (s Selector) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"state":   s.state,
		"version": s.version,
	})
}
