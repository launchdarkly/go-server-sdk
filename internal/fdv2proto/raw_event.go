package fdv2proto

import (
	"encoding/json"
)

type RawEvent struct {
	Name EventName       `json:"name"`
	Data json.RawMessage `json:"data"`
}
