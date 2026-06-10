package ldservices

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
	"github.com/launchdarkly/go-test-helpers/v3/jsonhelpers"
)

type fakeVersionedKind struct {
	Key     string `json:"key"`
	Version int    `json:"version"`
}

// KeyAndVersionItem provides a simple object that has only "key" and "version" properties.
// This may be enough for some testing purposes that don't require full flag or segment data.
func KeyAndVersionItem(key string, version int) interface{} {
	return fakeVersionedKind{Key: key, Version: version}
}

// ServerSDKData is a convenience type for constructing a test server-side SDK data payload for
// PollingServiceHandler or StreamingServiceHandler. Its String() method returns a JSON object with
// the expected "flags" and "segments" properties.
//
//	data := NewServerSDKData().Flags(flag1, flag2)
//	handler := PollingServiceHandler(data)
type ServerSDKData struct {
	FlagsMap    map[string]interface{} `json:"flags"`
	SegmentsMap map[string]interface{} `json:"segments"`
}

// NewServerSDKData creates a ServerSDKData instance.
func NewServerSDKData() *ServerSDKData {
	return &ServerSDKData{
		make(map[string]interface{}),
		make(map[string]interface{}),
	}
}

// String returns the JSON encoding of the struct as a string.
func (s *ServerSDKData) String() string {
	bytes, _ := json.Marshal(*s)
	return string(bytes)
}

// Flags adds the specified items to the struct's "flags" map.
//
// Each item may be either a object produced by KeyAndVersionItem or a real data model object from the ldmodel
// package. The minimum requirement is that when converted to JSON, it has a "key" property.
func (s *ServerSDKData) Flags(flags ...interface{}) *ServerSDKData {
	for _, flag := range flags {
		if key := getKeyFromJSON(flag); key != "" {
			s.FlagsMap[key] = flag
		}
	}
	return s
}

// Segments adds the specified items to the struct's "segments" map.
//
// Each item may be either a object produced by KeyAndVersionItem or a real data model object from the ldmodel
// package. The minimum requirement is that when converted to JSON, it has a "key" property.
func (s *ServerSDKData) Segments(segments ...interface{}) *ServerSDKData {
	for _, segment := range segments {
		if key := getKeyFromJSON(segment); key != "" {
			s.SegmentsMap[key] = segment
		}
	}
	return s
}

func getKeyFromJSON(item interface{}) string {
	return ldvalue.Parse(jsonhelpers.ToJSON(item)).GetByKey("key").StringValue()
}

// ToPutEvent creates an SSE event in the format that is used by the server-side SDK streaming endpoint.
func (s *ServerSDKData) ToPutEvent() httphelpers.SSEEvent {
	return httphelpers.SSEEvent{
		Event: "put",
		Data:  fmt.Sprintf(`{"path": "/", "data": %s}`, s),
	}
}
