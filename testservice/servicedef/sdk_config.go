package servicedef

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

type SDKConfigParams struct {
	Credential          string                              `json:"credential"`
	StartWaitTimeMS     ldtime.UnixMillisecondTime          `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                `json:"initCanFail,omitempty"`
	Streaming           *SDKConfigStreamingParams           `json:"streaming,omitempty"`
	Events              *SDKConfigEventParams               `json:"events,omitempty"`
	PersistentDataStore *SDKConfigPersistentDataStoreParams `json:"persistentDataStore,omitempty"`
	BigSegments         *SDKConfigBigSegmentsParams         `json:"bigSegments,omitempty"`
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
}

type SDKConfigPersistentDataStoreParams struct {
	CallbackURI string `json:"callbackURI"`
}

type SDKConfigBigSegmentsParams struct {
	CallbackURI          string                     `json:"callbackUri"`
	UserCacheSize        ldvalue.OptionalInt        `json:"userCacheSize,omitempty"`
	UserCacheTimeMS      ldtime.UnixMillisecondTime `json:"userCacheTimeMs,omitempty"`
	StatusPollIntervalMS ldtime.UnixMillisecondTime `json:"statusPollIntervalMs,omitempty"`
	StaleAfterMS         ldtime.UnixMillisecondTime `json:"staleAfterMs,omitempty"`
}
