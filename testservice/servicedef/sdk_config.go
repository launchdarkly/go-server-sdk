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
	BigSegments         *SDKConfigBigSegmentsParams         `json:"bigSegments,omitempty"`
	Tags                *SDKConfigTagsParams                `json:"tags,omitempty"`
	Hooks               *SDKConfigHooksParams               `json:"hooks,omitempty"`
	PersistentDataStore *SDKConfigPersistentDataStoreParams `json:"persistentDataStore,omitempty"`
	DataSystem          *DataSystem                         `json:"dataSystem,omitempty"`
	Overrides           *SDKConfigOverridesParams           `json:"overrides,omitempty"`
}

// SDKConfigOverridesParams configures a file-based flag override source.
type SDKConfigOverridesParams struct {
	FilePaths             []string `json:"filePaths"`
	DuplicateKeysHandling *string  `json:"duplicateKeysHandling,omitempty"`
	Watch                 *bool    `json:"watch,omitempty"`
	Poll                  *bool    `json:"poll,omitempty"`
	PollIntervalMS        *int     `json:"pollIntervalMs,omitempty"`
}

type SDKConfigServiceEndpointsParams struct {
	Streaming string `json:"streaming,omitempty"`
	Polling   string `json:"polling,omitempty"`
	Events    string `json:"events,omitempty"`
}

type DataStoreMode int

const (
	// DataStoreModeRead indicates that the data store is read-only. Data will never be written back to the store by
	// the SDK.
	DataStoreModeRead = 0
	// DataStoreModeReadWrite indicates that the data store is read-write. Data from initializers/synchronizers may be
	// written to the store as necessary.
	DataStoreModeReadWrite = 1
)

type DataSystem struct {
	Store         *DataStore              `json:"store,omitempty"`
	StoreMode     DataStoreMode           `json:"storeMode"`
	Initializers  []DataInitializer       `json:"initializers"`
	Synchronizers *Synchronizers          `json:"synchronizers,omitempty"`
	FDv1Fallback  *SDKConfigPollingParams `json:"fdv1Fallback,omitempty"`
	PayloadFilter *string                 `json:"payloadFilter,omitempty"`
}

type DataStore struct {
	PersistentDataStore *SDKConfigPersistentDataStoreParams `json:"persistentDataStore,omitempty"`
}

type DataInitializer struct {
	Polling *SDKConfigPollingParams `json:"polling,omitempty"`
}

type Synchronizers []Synchronizer

type Synchronizer struct {
	Streaming *SDKConfigStreamingParams `json:"streaming,omitempty"`
	Polling   *SDKConfigPollingParams   `json:"polling,omitempty"`
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

type SDKConfigPersistentDataStoreParams struct {
	Store SDKConfigPersistentStore `json:"store"`
	Cache SDKConfigPersistentCache `json:"cache"`
}

type SDKConfigPersistentType string

const (
	Redis    = SDKConfigPersistentType("redis")
	DynamoDB = SDKConfigPersistentType("dynamodb")
	Consul   = SDKConfigPersistentType("consul")
)

type SDKConfigPersistentStore struct {
	Type   SDKConfigPersistentType `json:"type"`
	Prefix *string                 `json:"prefix,omitempty"`
	DSN    string                  `json:"dsn"`
}

type SDKConfigPersistentMode string

const (
	Off      = SDKConfigPersistentMode("off")
	TTL      = SDKConfigPersistentMode("ttl")
	Infinite = SDKConfigPersistentMode("infinite")
)

type SDKConfigPersistentCache struct {
	Mode SDKConfigPersistentMode `json:"mode"`

	// This value is only valid when the Mode is set to TTL. It must be a positive integer.
	TTL *int `json:"ttl,omitempty"`
}
