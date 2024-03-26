package ldotel

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

const eventName = "feature_flag"
const contextKeyAttributeName = "feature_flag.context.key"

// TracingHookOption is used to implement functional options for the TracingHook.
type TracingHookOption func(hook *TracingHook)

// WithSpans option enables generation of child spans for each variation call.
func WithSpans() TracingHookOption {
	return func(h *TracingHook) {
		h.spans = true
	}
}

// WithVariant option enables putting a stringified version of the flag value in the feature_flag span event.
func WithVariant() TracingHookOption {
	return func(h *TracingHook) {
		h.includeVariant = true
	}
}

// A TracingHook adds OpenTelemetry support to the LaunchDarkly SDK.
//
// By default, span events will be added for each call to a "Variation" method. Variation methods without "Ex" will not
// be able to access a parent span, so no span events can be attached. If WithSpans is used, then root spans will be
// created from the non-"Ex" methods.
//
// The span event will include the FullyQualifiedKey of the ldcontext, the provider of the evaluation (LaunchDarkly),
// and the key of the flag being evaluated.
type TracingHook struct {
	ldhooks.UnimplementedHook
	metadata       ldhooks.HookMetadata
	spans          bool
	includeVariant bool
	tracer         trace.Tracer
}

// GetMetadata returns meta-data about the tracing hook.
func (h TracingHook) GetMetadata() ldhooks.HookMetadata {
	return h.metadata
}

// NewTracingHook creates a new TracingHook instance. The TracingHook can be provided to the LaunchDarkly client
// in order to add OpenTelemetry support.
func NewTracingHook(opts ...TracingHookOption) TracingHook {
	hook := TracingHook{
		metadata: ldhooks.NewHookMetadata("LaunchDarkly Tracing Hook"),
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

		span.SetAttributes(semconv.FeatureFlagKey(seriesContext.FlagKey()),
			attribute.String(contextKeyAttributeName, seriesContext.Context().FullyQualifiedKey()))

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
		semconv.FeatureFlagKey(seriesContext.FlagKey()),
		semconv.FeatureFlagProviderName("LaunchDarkly"),
		attribute.String(contextKeyAttributeName, seriesContext.Context().FullyQualifiedKey()),
	}
	if h.includeVariant {
		attribs = append(attribs, semconv.FeatureFlagVariant(detail.Value.JSONString()))
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent(eventName, trace.WithAttributes(attribs...))
	return data, nil
}

// Ensure that TracingHook conforms to the ldhooks.Hook interface.
var _ ldhooks.Hook = TracingHook{}
