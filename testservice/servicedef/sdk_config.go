package servicedef

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

type SDKConfigParams struct {
	Credential      string                     `json:"credential"`
	StartWaitTimeMS ldtime.UnixMillisecondTime `json:"startWaitTimeMs,omitempty"`
	InitCanFail     bool                       `json:"initCanFail,omitempty"`
	Streaming       *SDKConfigStreamingParams  `json:"streaming,omitempty"`
	Events          *SDKConfigEventParams      `json:"events,omitempty"`
}

type SDKConfigStreamingParams struct {
	BaseURI             string                      `json:"baseUri,omitempty"`
	InitialRetryDelayMs *ldtime.UnixMillisecondTime `json:"initialRetryDelayMs,omitempty"`
}

type SDKConfigEventParams struct {
	BaseURI                 string                     `json:"baseUri,omitempty"`
	Capacity                ldvalue.OptionalInt        `json:"capacity,omitempty"`
	EnableDiagnostics       bool                       `json:"enableDiagnostics"`
	AllAttributesPrivate    bool                       `json:"allAttributesPrivate,omitempty"`
	GlobalPrivateAttributes []lduser.UserAttribute     `json:"globalPrivateAttributes,omitempty"`
	FlushIntervalMS         ldtime.UnixMillisecondTime `json:"flushIntervalMs,omitempty"`
	InlineUsers             bool                       `json:"inlineUsers,omitempty"`
}
