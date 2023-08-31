package servicedef

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const (
	CommandEvaluateFlag             = "evaluate"
	CommandEvaluateAllFlags         = "evaluateAll"
	CommandIdentifyEvent            = "identifyEvent"
	CommandCustomEvent              = "customEvent"
	CommandAliasEvent               = "aliasEvent"
	CommandFlushEvents              = "flushEvents"
	CommandGetBigSegmentStoreStatus = "getBigSegmentStoreStatus"
	CommandContextBuild             = "contextBuild"
	CommandContextConvert           = "contextConvert"
	CommandSecureModeHash           = "secureModeHash"
	CommandMigrationVariation       = "migrationVariation"
	CommandMigrationOperation       = "migrationOperation"
)

type ValueType string

const (
	ValueTypeBool   = "bool"
	ValueTypeInt    = "int"
	ValueTypeDouble = "double"
	ValueTypeString = "string"
	ValueTypeAny    = "any"
)

type CommandParams struct {
	Command            string                    `json:"command"`
	Evaluate           *EvaluateFlagParams       `json:"evaluate,omitempty"`
	EvaluateAll        *EvaluateAllFlagsParams   `json:"evaluateAll,omitempty"`
	CustomEvent        *CustomEventParams        `json:"customEvent,omitempty"`
	IdentifyEvent      *IdentifyEventParams      `json:"identifyEvent,omitempty"`
	ContextBuild       *ContextBuildParams       `json:"contextBuild,omitempty"`
	ContextConvert     *ContextConvertParams     `json:"contextConvert,omitempty"`
	SecureModeHash     *SecureModeHashParams     `json:"secureModeHash,omitempty"`
	MigrationVariation *MigrationVariationParams `json:"migrationVariation,omitempty"`
	MigrationOperation *MigrationOperationParams `json:"migrationOperation,omitempty"`
}

type EvaluateFlagParams struct {
	FlagKey      string            `json:"flagKey"`
	Context      ldcontext.Context `json:"context"`
	ValueType    ValueType         `json:"valueType"`
	DefaultValue ldvalue.Value     `json:"defaultValue"`
	Detail       bool              `json:"detail"`
}

type EvaluateFlagResponse struct {
	Value          ldvalue.Value              `json:"value"`
	VariationIndex *int                       `json:"variationIndex,omitempty"`
	Reason         *ldreason.EvaluationReason `json:"reason,omitempty"`
}

type EvaluateAllFlagsParams struct {
	Context                    ldcontext.Context `json:"context"`
	WithReasons                bool              `json:"withReasons"`
	ClientSideOnly             bool              `json:"clientSideOnly"`
	DetailsOnlyForTrackedFlags bool              `json:"detailsOnlyForTrackedFlags"`
}

type EvaluateAllFlagsResponse struct {
	State map[string]ldvalue.Value `json:"state"`
}

type CustomEventParams struct {
	EventKey     string            `json:"eventKey"`
	Context      ldcontext.Context `json:"context"`
	Data         ldvalue.Value     `json:"data,omitempty"`
	OmitNullData bool              `json:"omitNullData"`
	MetricValue  *float64          `json:"metricValue,omitempty"`
}

type IdentifyEventParams struct {
	Context ldcontext.Context `json:"context"`
}

type BigSegmentStoreStatusResponse struct {
	Available bool `json:"available"`
	Stale     bool `json:"stale"`
}

type ContextBuildParams struct {
	Single *ContextBuildSingleParams  `json:"single,omitempty"`
	Multi  []ContextBuildSingleParams `json:"multi,omitempty"`
}

type ContextBuildSingleParams struct {
	Kind      *string                  `json:"kind,omitempty"`
	Key       string                   `json:"key"`
	Name      *string                  `json:"name,omitempty"`
	Anonymous *bool                    `json:"anonymous,omitempty"`
	Secondary *string                  `json:"secondary,omitempty"`
	Private   []string                 `json:"private,omitempty"`
	Custom    map[string]ldvalue.Value `json:"custom,omitempty"`
}

type ContextBuildResponse struct {
	Output string `json:"output"`
	Error  string `json:"error"`
}

type ContextConvertParams struct {
	Input string `json:"input"`
}

type SecureModeHashParams struct {
	Context ldcontext.Context `json:"context"`
}

type SecureModeHashResponse struct {
	Result string `json:"result"`
}

type MigrationVariationParams struct {
	Key          string            `json:"key"`
	Context      ldcontext.Context `json:"context"`
	DefaultStage ldmigration.Stage `json:"defaultStage"`
}

type MigrationVariationResponse struct {
	Result string `json:"result"`
}

type MigrationOperationParams struct {
	Key                string                     `json:"key"`
	Context            ldcontext.Context          `json:"context"`
	DefaultStage       ldmigration.Stage          `json:"defaultStage"`
	ReadExecutionOrder ldmigration.ExecutionOrder `json:"readExecutionOrder"`
	Operation          ldmigration.Operation      `json:"operation"`
	OldEndpoint        string                     `json:"oldEndpoint"`
	NewEndpoint        string                     `json:"newEndpoint"`
	Payload            *string                    `json:"payload,omitempty"`
	TrackLatency       bool                       `json:"trackLatency"`
	TrackErrors        bool                       `json:"trackErrors"`
	TrackConsistency   bool                       `json:"trackConsistency"`
}

type MigrationOperationResponse struct {
	Result interface{} `json:"result"`
}
