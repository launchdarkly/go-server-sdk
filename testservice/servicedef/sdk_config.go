package servicedef

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

type SDKConfigParams struct {
	Credential          string                              `json:"credential"`
	StartWaitTimeMS     ldtime.UnixMillisecondTime          `json:"startWaitTimeMs,omitempty"`
	InitCanFail         bool                                `json:"initCanFail,omitempty"`
	ServiceEndpoints    *SDKConfigServiceEndpointsParams    `json:"serviceEndpoints,omitempty"`
	Streaming           *SDKConfigStreamingParams           `json:"streaming,omitempty"`
	Polling             *SDKConfigPollingParams             `json:"polling,omitempty"`
	Events              *SDKConfigEventParams               `json:"events,omitempty"`
	PersistentDataStore *SDKConfigPersistentDataStoreParams `json:"persistentDataStore,omitempty"`
	BigSegments         *SDKConfigBigSegmentsParams         `json:"bigSegments,omitempty"`
	Tags                *SDKConfigTagsParams                `json:"tags,omitempty"`
	Hooks               *SDKConfigHooksParams               `json:"hooks,omitempty"`
}

type SDKConfigServiceEndpointsParams struct {
	Streaming string `json:"streaming,omitempty"`
	Polling   string `json:"polling,omitempty"`
	Events    string `json:"events,omitempty"`
}

type SDKConfigStreamingParams struct {
	BaseURI             string                      `json:"baseUri,omitempty"`
	InitialRetryDelayMS *ldtime.UnixMillisecondTime `json:"initialRetryDelayMs,omitempty"`
	Filter              ldvalue.OptionalString      `json:"filter,omitempty"`
}

type SDKConfigPollingParams struct {
	BaseURI        string                      `json:"baseUri,omitempty"`
	PollIntervalMS *ldtime.UnixMillisecondTime `json:"pollIntervalMs,omitempty"`
	Filter         ldvalue.OptionalString      `json:"filter,omitempty"`
}

type SDKConfigEventParams struct {
	BaseURI                 string                     `json:"baseUri,omitempty"`
	Capacity                ldvalue.OptionalInt        `json:"capacity,omitempty"`
	EnableDiagnostics       bool                       `json:"enableDiagnostics"`
	AllAttributesPrivate    bool                       `json:"allAttributesPrivate,omitempty"`
	GlobalPrivateAttributes []string                   `json:"globalPrivateAttributes,omitempty"`
	FlushIntervalMS         ldtime.UnixMillisecondTime `json:"flushIntervalMs,omitempty"`
	OmitAnonymousContexts   bool                       `json:"omitAnonymousContexts,omitempty"`
	EnableGzip              ldvalue.OptionalBool       `json:"enableGzip,omitempty"`
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

type SDKConfigTagsParams struct {
	ApplicationID      ldvalue.OptionalString `json:"applicationId,omitempty"`
	ApplicationVersion ldvalue.OptionalString `json:"applicationVersion,omitempty"`
}

type SDKConfigEvaluationHookData map[string]ldvalue.Value

type SDKConfigHookInstance struct {
	Name        string                                    `json:"name"`
	CallbackURI string                                    `json:"callbackUri"`
	Data        map[HookStage]SDKConfigEvaluationHookData `json:"data,omitempty"`
	Errors      map[HookStage]string                      `json:"errors,omitempty"`
}

type SDKConfigHooksParams struct {
	Hooks []SDKConfigHookInstance `json:"hooks"`
}
