package ldservicesv2

import (
	"encoding/json"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
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
	FlagsMap    map[string]ldmodel.FeatureFlag `json:"flags"`
	SegmentsMap map[string]ldmodel.Segment     `json:"segments"`
}

// NewServerSDKData creates a ServerSDKData instance.
func NewServerSDKData() *ServerSDKData {
	return &ServerSDKData{
		make(map[string]ldmodel.FeatureFlag),
		make(map[string]ldmodel.Segment),
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
func (s *ServerSDKData) Flags(flags ...ldmodel.FeatureFlag) *ServerSDKData {
	for _, flag := range flags {
		s.FlagsMap[flag.Key] = flag
	}
	return s
}

// Segments adds the specified items to the struct's "segments" map.
//
// Each item may be either a object produced by KeyAndVersionItem or a real data model object from the ldmodel
// package. The minimum requirement is that when converted to JSON, it has a "key" property.
func (s *ServerSDKData) Segments(segments ...ldmodel.Segment) *ServerSDKData {
	for _, segment := range segments {
		s.SegmentsMap[segment.Key] = segment
	}
	return s
}

func (s *ServerSDKData) ToPutObjects() []fdv2proto.PutObject {
	var objs []fdv2proto.PutObject
	for _, flag := range s.FlagsMap {
		base := fdv2proto.PutObject{
			Version: flag.Version,
			Kind:    fdv2proto.FlagKind,
			Key:     flag.Key,
			Object:  flag,
		}
		objs = append(objs, base)
	}
	for _, segment := range s.SegmentsMap {
		base := fdv2proto.PutObject{
			Version: segment.Version,
			Kind:    fdv2proto.SegmentKind,
			Key:     segment.Key,
			Object:  segment,
		}
		objs = append(objs, base)
	}
	return objs
}
