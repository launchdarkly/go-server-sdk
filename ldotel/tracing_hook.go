package ldotel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/launchdarkly/go-sdk-common/v4/ldreason"
	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

const eventName = "feature_flag"

const attrFeatureFlagKey = "feature_flag.key"
const attrFeatureFlagContextID = "feature_flag.context.id"
const attrFeatureFlagProviderName = "feature_flag.provider.name"
const attrFeatureFlagResultValue = "feature_flag.result.value"
const attrFeatureFlagReasonInExperiment = "feature_flag.result.reason.inExperiment"
const attrFeatureFlagResultVariationIndex = "feature_flag.result.variationIndex"
const attrFeatureFlagSetID = "feature_flag.set.id"

func featureFlagKey(key string) attribute.KeyValue {
	return attribute.String(attrFeatureFlagKey, key)
}

func featureFlagContextID(contextID string) attribute.KeyValue {
	return attribute.String(attrFeatureFlagContextID, contextID)
}

func featureFlagProviderName(providerName string) attribute.KeyValue {
	return attribute.String(attrFeatureFlagProviderName, providerName)
}

func featureFlagResultValue(value string) attribute.KeyValue {
	return attribute.String(attrFeatureFlagResultValue, value)
}

func featureFlagReasonInExperiment(inExperiment bool) attribute.KeyValue {
	return attribute.Bool(attrFeatureFlagReasonInExperiment, inExperiment)
}

func featureFlagResultVariationIndex(variationIndex int) attribute.KeyValue {
	return attribute.Int(attrFeatureFlagResultVariationIndex, variationIndex)
}

func featureFlagSetID(setID string) attribute.KeyValue {
	return attribute.String(attrFeatureFlagSetID, setID)
}

// TracingHookOption is used to implement functional options for the TracingHook.
type TracingHookOption func(hook *TracingHook)

// WithSpans is an experimental option that enables creation of child spans for each variation call.
//
// This feature is experimental and the data in the spans, or nesting of spans, could change in future versions.
func WithSpans() TracingHookOption {
	return func(h *TracingHook) {
		h.spans = true
	}
}

// WithVariant option enables putting a stringified version of the flag value in the feature_flag span event.
//
// Deprecated: Use WithValue instead.
func WithVariant() TracingHookOption {
	return func(h *TracingHook) {
		h.includeValue = true
	}
}

// WithValue option enables putting a stringified version of the flag value in the feature_flag span event.
func WithValue() TracingHookOption {
	return func(h *TracingHook) {
		h.includeValue = true
	}
}

// WithEnvironmentID option sets the environemntID to be used as feature_flag.set.id, which will override any
// environmentID supplied by the hook series context.
func WithEnvironmentID(environmentID string) TracingHookOption {
	return func(h *TracingHook) {
		h.environmentID = ldvalue.NewOptionalString(environmentID)
	}
}

// A TracingHook adds OpenTelemetry support to the LaunchDarkly SDK.
//
// By default, span events will be added for each call to a "Variation" method. Variation methods without "Ctx" will not
// be able to access a parent span, so no span events can be attached. If WithSpans is used, then root spans will be
// created from the non-"Ctx" methods.
//
// The span event will include the FullyQualifiedKey of the ldcontext, the provider of the evaluation (LaunchDarkly),
// and the key of the flag being evaluated.
type TracingHook struct {
	ldhooks.Unimplemented
	metadata      ldhooks.Metadata
	spans         bool
	includeValue  bool
	tracer        trace.Tracer
	environmentID ldvalue.OptionalString
}

// Metadata returns meta-data about the tracing hook.
func (h TracingHook) Metadata() ldhooks.Metadata {
	return h.metadata
}

// NewTracingHook creates a new TracingHook instance. The TracingHook can be provided to the LaunchDarkly client
// in order to add OpenTelemetry support.
func NewTracingHook(opts ...TracingHookOption) TracingHook {
	hook := TracingHook{
		metadata: ldhooks.NewMetadata("LaunchDarkly Tracing Hook"),
		tracer:   otel.Tracer("launchdarkly-client"),
	}
	for _, opt := range opts {
		opt(&hook)
	}
	return hook
}

// BeforeEvaluation implements the BeforeEvaluation evaluation stage.
func (h TracingHook) BeforeEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
	if h.spans {
		_, span := h.tracer.Start(ctx, seriesContext.Method())

		span.SetAttributes(featureFlagKey(seriesContext.FlagKey()),
			featureFlagContextID(seriesContext.Context().FullyQualifiedKey()),
		)

		return ldhooks.NewEvaluationSeriesBuilder(data).Set("variationSpan", span).Build(), nil
	}
	return data, nil
}

// AfterEvaluation implements the AfterEvaluation evaluation stage.
func (h TracingHook) AfterEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error) {
	variationSpan, present := data.Get("variationSpan")
	if present {
		asSpan, ok := variationSpan.(trace.Span)
		if ok {
			asSpan.End()
		}
	}

	attribs := []attribute.KeyValue{
		featureFlagKey(seriesContext.FlagKey()),
		featureFlagProviderName("LaunchDarkly"),
		featureFlagContextID(seriesContext.Context().FullyQualifiedKey()),
	}
	if h.includeValue {
		attribs = append(attribs, featureFlagResultValue(detail.Value.JSONString()))
	}

	if detail.Reason.IsInExperiment() {
		attribs = append(attribs, featureFlagReasonInExperiment(detail.Reason.IsInExperiment()))
	}

	if detail.VariationIndex.IsDefined() {
		attribs = append(attribs, featureFlagResultVariationIndex(detail.VariationIndex.IntValue()))
	}

	if id, ok := h.environmentID.Get(); ok {
		attribs = append(attribs, featureFlagSetID(id))
	} else if id, ok := seriesContext.EnvironmentID().Get(); ok {
		attribs = append(attribs, featureFlagSetID(id))
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent(eventName, trace.WithAttributes(attribs...))
	return data, nil
}

// Ensure that TracingHook conforms to the ldhooks.Hook interface.
var _ ldhooks.Hook = TracingHook{}
