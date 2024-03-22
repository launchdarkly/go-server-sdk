package ldotel

import (
	"context"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"
)

const eventName = "feature_flag"
const contextKeyAttributeName = "feature_flag.context.key"

type TracingHookOption func(hook *TracingHook)

var tracer = otel.Tracer("launchdarkly-client")

// The WithSpans option enables generation of child spans for each variation call.
func WithSpans() TracingHookOption {
	return func(h *TracingHook) {
		h.spans = true
	}
}

// The WithVariant option enables putting a stringified version of the flag value in the feature_flag span event.
func WithVariant() TracingHookOption {
	return func(h *TracingHook) {
		h.includeVariant = true
	}
}

// A TracingHook adds OpenTelemetry support to the LaunchDarkly SDK.
//
// By default, span events will be added for each call to a "VariationEx" method.
// The span event will include the FullyQualifiedKey of the ldcontext, the provider of the evaluation (LaunchDarkly),
// and the key of the flag being evaluated.
type TracingHook struct {
	ldhooks.UnimplementedHook
	metadata       ldhooks.HookMetadata
	spans          bool
	includeVariant bool
}

// NewTracingHook creates a new TracingHook instance. The TracingHook can be provided to the LaunchDarkly client
// in order to add OpenTelemetry support.
func NewTracingHook(opts ...TracingHookOption) TracingHook {
	hook := TracingHook{
		metadata: ldhooks.NewHookMetadata("LaunchDarkly Tracing Hook"),
	}
	for _, opt := range opts {
		opt(&hook)
	}
	return hook
}

func (h TracingHook) GetMetaData() ldhooks.HookMetadata {
	return h.metadata
}

func (h TracingHook) BeforeEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData) ldhooks.EvaluationSeriesData {
	if h.spans {
		_, span := tracer.Start(ctx, seriesContext.Method())

		span.SetAttributes(semconv.FeatureFlagKey(seriesContext.FlagKey()),
			attribute.String(contextKeyAttributeName, seriesContext.Context().FullyQualifiedKey()))

		return ldhooks.NewEvaluationSeriesBuilder(data).Set("variationSpan", span).Build()
	}
	return data
}

func (h TracingHook) AfterEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) ldhooks.EvaluationSeriesData {
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
	return data
}

// Ensure that TracingHook conforms to the ldhooks.Hook interface.
var _ ldhooks.Hook = TracingHook{}
