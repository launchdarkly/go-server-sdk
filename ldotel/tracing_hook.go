package ldotel

import (
	"context"

	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.18.0"
	"go.opentelemetry.io/otel/trace"
)

const eventName = "feature_flag"
const contextKeyAttributeName = "feature_flag.context.key"

type TracingHook struct {
	ldhooks.UnimplementedHook
	metadata ldhooks.HookMetadata
}

func NewTracingHook() TracingHook {
	return TracingHook{
		metadata: ldhooks.NewHookMetadata("LaunchDarkly Tracing Hook"),
	}
}

func (h TracingHook) GetMetaData() ldhooks.HookMetadata {
	return h.metadata
}

func (h TracingHook) AfterEvaluation(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext, data ldhooks.EvaluationSeriesData) ldhooks.EvaluationSeriesData {
	attribs := []attribute.KeyValue{
		semconv.FeatureFlagKey(seriesContext.FlagKey()),
		semconv.FeatureFlagProviderName("LaunchDarkly"),
		attribute.String(contextKeyAttributeName, seriesContext.Context().FullyQualifiedKey()),
	}

	span := trace.SpanFromContext(ctx)
	span.AddEvent(eventName, trace.WithAttributes(attribs...))
	return ldhooks.NewEvaluationSeriesBuilder(data).Set("my-test-data", "my-test-value").Build()
}
