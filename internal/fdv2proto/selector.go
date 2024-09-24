package fdv2proto

import (
	"encoding/json"
	"errors"
)

type Selector struct {
	state   string
	version int
}

func NoSelector() *Selector {
	return nil
}

func NewSelector(state string, version int) *Selector {
	return &Selector{state: state, version: version}
}

func (s *Selector) IsSet() bool {
	return s != nil
}

func (s *Selector) State() string {
	return s.state
}

func (s *Selector) Version() int {
	return s.version
}

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
