package ldservicesv2

import (
	"encoding/json"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

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

func mustMarshal(model any) json.RawMessage {
	data, err := json.Marshal(model)
	if err != nil {
		panic(err)
	}
	return data
}

// ToInitializerPayload converts the data to a PollingPayload object that can
// be fed to a mock polling service.
func (s *ServerSDKData) ToInitializerPayload() subsystems.PollingPayload {
	pollingPayload := subsystems.PollingPayload{}
	pollingPayload.Events = make([]subsystems.RawEvent, 0, 10)

	pollingPayload.Events = append(pollingPayload.Events, subsystems.RawEvent{
		Name: "server-intent",
		Data: mustMarshal(subsystems.ServerIntent{
			Payload: subsystems.Payload{
				ID:     "some-id",
				Target: 0,
				Code:   subsystems.IntentTransferFull,
				Reason: "cant-catchup",
			},
		}),
	})

	for _, putObject := range s.ToPutObjects() {
		pollingPayload.Events = append(pollingPayload.Events, subsystems.RawEvent{
			Name: "put-object",
			Data: mustMarshal(putObject),
		})
	}

	pollingPayload.Events = append(pollingPayload.Events, subsystems.RawEvent{
		Name: "payload-transferred",
		Data: mustMarshal(subsystems.NewSelector("(p:17YNC7XBH88Y6RDJJ48EKPCJS7:53)", 1)),
	})

	return pollingPayload
}

// ToPutObjects converts the data to a list of PutObject objects that can be fed to a mock streaming data source.
func (s *ServerSDKData) ToPutObjects() []subsystems.PutObject {
	objs := make([]subsystems.PutObject, 0, len(s.FlagsMap)+len(s.SegmentsMap))
	for _, flag := range s.FlagsMap {
		base := subsystems.PutObject{
			Version: flag.Version,
			Kind:    subsystems.FlagKind,
			Key:     flag.Key,
			Object:  mustMarshal(flag),
		}
		objs = append(objs, base)
	}
	for _, segment := range s.SegmentsMap {
		base := subsystems.PutObject{
			Version: segment.Version,
			Kind:    subsystems.SegmentKind,
			Key:     segment.Key,
			Object:  mustMarshal(segment),
		}
		objs = append(objs, base)
	}
	return objs
}
