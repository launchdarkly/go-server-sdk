package servicedef

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"

	CapabilityAllFlagsWithReasons                = "all-flags-with-reasons"
	CapabilityAllFlagsClientSideOnly             = "all-flags-client-side-only"
	CapabilityAllFlagsDetailsOnlyForTrackedFlags = "all-flags-details-only-for-tracked-flags"

	CapabilityBigSegments                 = "big-segments"
	CapabilitySecureModeHash              = "secure-mode-hash"
	CapabilityServerSidePolling           = "server-side-polling"
	CapabilityServiceEndpoints            = "service-endpoints"
	CapabilityTags                        = "tags"
	CapabilityFiltering                   = "filtering"
	CapabilityContextType                 = "context-type"
	CapabilityMigrations                  = "migrations"
	CapabilityEventSampling               = "event-sampling"
	CapabilityInlineContextAll            = "inline-context-all"
	CapabilityAnonymousRedaction          = "anonymous-redaction"
	CapabilityEvaluationHooks             = "evaluation-hooks"
	CapabilityTrackHooks                  = "track-hooks"
	CapabilityOmitAnonymousContexts       = "omit-anonymous-contexts"
	CapabilityEventGzip                   = "event-gzip"
	CapabilityOptionalEventGzip           = "optional-event-gzip"
	CapabilityClientPrereqEvents          = "client-prereq-events"
	CapabilityPersistentDataStoreRedis    = "persistent-data-store-redis"
	CapabilityPersistentDataStoreConsul   = "persistent-data-store-consul"
	CapabilityPersistentDataStoreDynamoDB = "persistent-data-store-dynamodb"
	CapabilityFlagChangeListeners         = "flag-change-listeners"
	CapabilityFlagValueChangeListeners    = "flag-value-change-listeners"
	CapabilityFDv1Fallback                = "fdv1-fallback"
	CapabilityInstanceID                  = "instance-id"
	CapabilityFlagOverrides               = "flag-overrides"
)

type StatusRep struct {
	// Name is the name of the project that the test service is testing, such as "go-server-sdk".
	Name string `json:"name"`

	// Capabilities is a list of strings representing optional features of the test service.
	Capabilities []string `json:"capabilities"`

	ClientVersion string `json:"clientVersion"`
}

type CreateInstanceParams struct {
	Configuration SDKConfigParams `json:"configuration"`
	Tag           string          `json:"tag"`
}
