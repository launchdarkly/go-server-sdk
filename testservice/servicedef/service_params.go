package servicedef

const (
	CapabilityClientSide    = "client-side"
	CapabilityServerSide    = "server-side"
	CapabilityStronglyTyped = "strongly-typed"

	CapabilityAllFlagsWithReasons                = "all-flags-with-reasons"
	CapabilityAllFlagsClientSideOnly             = "all-flags-client-side-only"
	CapabilityAllFlagsDetailsOnlyForTrackedFlags = "all-flags-details-only-for-tracked-flags"

	CapabilityBigSegments           = "big-segments"
	CapabilitySecureModeHash        = "secure-mode-hash"
	CapabilityServerSidePolling     = "server-side-polling"
	CapabilityServiceEndpoints      = "service-endpoints"
	CapabilityTags                  = "tags"
	CapabilityFiltering             = "filtering"
	CapabilityContextType           = "context-type"
	CapabilityMigrations            = "migrations"
	CapabilityEventSampling         = "event-sampling"
	CapabilityInlineContext         = "inline-context"
	CapabilityAnonymousRedaction    = "anonymous-redaction"
	CapabilityEvaluationHooks       = "evaluation-hooks"
	CapabilityOmitAnonymousContexts = "omit-anonymous-contexts"
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
