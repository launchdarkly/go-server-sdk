package ldotel

import (
	gocontext "context"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	ldclient "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
)
import "go.opentelemetry.io/otel/sdk/trace/tracetest"

func configureMemoryExporter() *tracetest.InMemoryExporter {
	exporter := tracetest.NewInMemoryExporter()
	sp := trace.NewSimpleSpanProcessor(exporter)
	provider := trace.NewTracerProvider(
		trace.WithSpanProcessor(sp),
	)
	otel.SetTracerProvider(provider)
	return exporter
}

func createClientWithTracing(options ...TracingHookOption) *ldclient.LDClient {
	client, _ := ldclient.MakeCustomClient("", ldclient.Config{
		Offline: true,
		Hooks:   []ldhooks.Hook{NewTracingHook(options...)},
	}, 0)
	return client
}

const flagKey = "test-flag"
const spanName = "test-span"

func TestBasicSpanEventsEvents(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client := createClientWithTracing()
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 1)
	events := exportedSpans[0].Events()
	assert.Len(t, events, 1)
	flagEvent := events[0]
	assert.Equal(t, "feature_flag", flagEvent.Name)

	attributes := attribute.NewSet(flagEvent.Attributes...)
	attributeFlagKey, _ := (&attributes).Value("feature_flag.key")
	assert.Equal(t, flagKey, attributeFlagKey.AsString())
	attributeProviderName, _ := (&attributes).Value("feature_flag.provider_name")
	assert.Equal(t, "LaunchDarkly", attributeProviderName.AsString())
	attributeContextKey, _ := (&attributes).Value("feature_flag.context.key")
	assert.Equal(t, context.FullyQualifiedKey(), attributeContextKey.AsString())
}

func TestSpanEventsWithVariant(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client := createClientWithTracing(WithVariant())
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	events := exportedSpans[0].Events()
	flagEvent := events[0]

	attributes := attribute.NewSet(flagEvent.Attributes...)
	attributeVariant, _ := (&attributes).Value("feature_flag.variant")
	assert.Equal(t, "false", attributeVariant.AsString())
}

func TestMultipleSpanEvents(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client := createClientWithTracing()
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)
	_, _ = client.StringVariationCtx(ctx, flagKey, context, "default")

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 1)
	events := exportedSpans[0].Events()
	assert.Len(t, events, 2)
	flagEventBool := events[0]
	assert.Equal(t, "feature_flag", flagEventBool.Name)

	boolFlagEventAttributes := attribute.NewSet(flagEventBool.Attributes...)
	boolAttributeFlagKey, _ := (&boolFlagEventAttributes).Value("feature_flag.key")
	assert.Equal(t, flagKey, boolAttributeFlagKey.AsString())
	boolAttributeProviderName, _ := (&boolFlagEventAttributes).Value("feature_flag.provider_name")
	assert.Equal(t, "LaunchDarkly", boolAttributeProviderName.AsString())
	boolAttributeContextKey, _ := (&boolFlagEventAttributes).Value("feature_flag.context.key")
	assert.Equal(t, context.FullyQualifiedKey(), boolAttributeContextKey.AsString())

	flagEventString := events[1]
	assert.Equal(t, "feature_flag", flagEventString.Name)

	stringFlagEventAttributes := attribute.NewSet(flagEventString.Attributes...)
	stringAttributeFlagKey, _ := (&stringFlagEventAttributes).Value("feature_flag.key")
	assert.Equal(t, flagKey, stringAttributeFlagKey.AsString())
	stringAttributeProviderName, _ := (&boolFlagEventAttributes).Value("feature_flag.provider_name")
	assert.Equal(t, "LaunchDarkly", stringAttributeProviderName.AsString())
	stringAttributeContextKey, _ := (&stringFlagEventAttributes).Value("feature_flag.context.key")
	assert.Equal(t, context.FullyQualifiedKey(), stringAttributeContextKey.AsString())
}

func TestSpanCreationWithParent(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client := createClientWithTracing(WithSpans())
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 2)

	exportedSpan := exportedSpans[0]
	assert.Equal(t, "LDClient.BoolVariationCtx", exportedSpan.Name())

	attributes := attribute.NewSet(exportedSpan.Attributes()...)
	attributeFlagKey, _ := (&attributes).Value("feature_flag.key")
	assert.Equal(t, flagKey, attributeFlagKey.AsString())
	attributeContextKey, _ := (&attributes).Value("feature_flag.context.key")
	assert.Equal(t, context.FullyQualifiedKey(), attributeContextKey.AsString())
}

func TestSpanCreationWithoutParent(t *testing.T) {
	exporter := configureMemoryExporter()
	client := createClientWithTracing(WithSpans())
	context := ldcontext.New("test-context")

	_, _ = client.BoolVariation(flagKey, context, false)

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 1)
	exportedSpan := exportedSpans[0]
	assert.Equal(t, "LDClient.BoolVariation", exportedSpan.Name())
}
