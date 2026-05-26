package ldotel

import (
	gocontext "context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v4/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v4/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservices"
	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldtestdata"
	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"

	"github.com/launchdarkly/go-sdk-common/v4/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v4/ldvalue"
	ldclient "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func configureMemoryExporter() *tracetest.InMemoryExporter {
	exporter := tracetest.NewInMemoryExporter()
	sp := trace.NewSimpleSpanProcessor(exporter)
	provider := trace.NewTracerProvider(
		trace.WithSpanProcessor(sp),
	)
	otel.SetTracerProvider(provider)
	return exporter
}

func createClientWithTracing(options ...TracingHookOption) (*ldclient.LDClient, *ldtestdata.TestDataSource) {
	td := ldtestdata.DataSource()
	client, _ := ldclient.MakeCustomClient("", ldclient.Config{
		Events:     ldcomponents.NoEvents(),
		DataSource: td,
		Hooks:      []ldhooks.Hook{NewTracingHook(options...)},
	}, 0)
	return client, td
}

func createStreamHandlerWithEnvironmentID(environmentID string) http.Handler {
	flag := ldbuilders.NewFlagBuilder(flagKey).SingleVariation(ldvalue.Bool(true)).Build()
	initialEvent := ldservices.NewServerSDKData().Flags(&flag).ToPutEvent()
	handler, _ := httphelpers.SSEHandlerWithEnvironmentID(&initialEvent, environmentID)
	return httphelpers.HandlerForPath("/all", httphelpers.HandlerForMethod("GET", handler, nil), nil)
}

func createClientWithStreamServerAndTracing(server *httptest.Server, options ...TracingHookOption) *ldclient.LDClient {
	client, _ := ldclient.MakeCustomClient("", ldclient.Config{
		Events:           ldcomponents.NoEvents(),
		ServiceEndpoints: interfaces.ServiceEndpoints{Streaming: server.URL},
		Hooks:            []ldhooks.Hook{NewTracingHook(options...)},
	}, 5*time.Second)
	return client
}

const flagKey = "test-flag"
const spanName = "test-span"

func TestBasicSpanEventsEvents(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing()
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
	attributeProviderName, _ := (&attributes).Value("feature_flag.provider.name")
	assert.Equal(t, "LaunchDarkly", attributeProviderName.AsString())
	attributeContextKey, _ := (&attributes).Value("feature_flag.context.id")
	assert.Equal(t, context.FullyQualifiedKey(), attributeContextKey.AsString())
}

func TestSpanEventsWithVariant(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing(WithVariant())
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	events := exportedSpans[0].Events()
	flagEvent := events[0]

	attributes := attribute.NewSet(flagEvent.Attributes...)
	attributeVariant, _ := (&attributes).Value("feature_flag.result.value")
	assert.Equal(t, "false", attributeVariant.AsString())
}

func TestMultipleSpanEvents(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing()
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
	boolAttributeProviderName, _ := (&boolFlagEventAttributes).Value("feature_flag.provider.name")
	assert.Equal(t, "LaunchDarkly", boolAttributeProviderName.AsString())
	boolAttributeContextKey, _ := (&boolFlagEventAttributes).Value("feature_flag.context.id")
	assert.Equal(t, context.FullyQualifiedKey(), boolAttributeContextKey.AsString())

	flagEventString := events[1]
	assert.Equal(t, "feature_flag", flagEventString.Name)

	stringFlagEventAttributes := attribute.NewSet(flagEventString.Attributes...)
	stringAttributeFlagKey, _ := (&stringFlagEventAttributes).Value("feature_flag.key")
	assert.Equal(t, flagKey, stringAttributeFlagKey.AsString())
	stringAttributeProviderName, _ := (&stringFlagEventAttributes).Value("feature_flag.provider.name")
	assert.Equal(t, "LaunchDarkly", stringAttributeProviderName.AsString())
	stringAttributeContextKey, _ := (&stringFlagEventAttributes).Value("feature_flag.context.id")
	assert.Equal(t, context.FullyQualifiedKey(), stringAttributeContextKey.AsString())
}

func TestSpanCreationWithParent(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing(WithSpans())
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
	attributeContextKey, _ := (&attributes).Value("feature_flag.context.id")
	assert.Equal(t, context.FullyQualifiedKey(), attributeContextKey.AsString())
}

func TestSpanCreationWithoutParent(t *testing.T) {
	exporter := configureMemoryExporter()
	client, _ := createClientWithTracing(WithSpans())
	context := ldcontext.New("test-context")

	_, _ = client.BoolVariation(flagKey, context, false)

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 1)
	exportedSpan := exportedSpans[0]
	assert.Equal(t, "LDClient.BoolVariation", exportedSpan.Name())
}

func TestSpanEventsWithInExperiment(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, td := createClientWithTracing()
	context := ldcontext.New("test-context")

	flagJSON := `{
		"key": "test-flag",
		"salt": "salty",
		"on": true,
		"fallthrough": {
			"rollout": {
				"kind": "experiment",
				"seed": 12345,
				"variations": [
					{
						"variation": 0,
						"weight": 100000
					},
					{
						"variation": 1,
						"weight": 0
					}
				]
			}
		},
		"variations": [
			true,
			false
		]
	}`

	flag, err := ldmodel.NewJSONDataModelSerialization().UnmarshalFeatureFlag([]byte(flagJSON))
	assert.NoError(t, err)

	td.UsePreconfiguredFlag(flag)

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
	attributeInExperiment, _ := (&attributes).Value("feature_flag.result.reason.inExperiment")
	assert.Equal(t, true, attributeInExperiment.AsBool())
}

func TestSpanEventsWithoutInExperiment(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, td := createClientWithTracing()
	context := ldcontext.New("test-context")

	// Create a flag that will NOT trigger an experiment
	td.Update(td.Flag(flagKey).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		FallthroughVariationIndex(1))

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
	_, found := (&attributes).Value("feature_flag.result.reason.inExperiment")
	// The attribute should not be present when not in experiment
	assert.False(t, found)
}

func TestSpanEventsWithVariationIndex(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, td := createClientWithTracing()
	context := ldcontext.New("test-context")

	td.Update(td.Flag(flagKey).On(true).
		Variations(ldvalue.String("variation0"), ldvalue.String("variation1"), ldvalue.String("variation2")).
		FallthroughVariationIndex(2))

	ctx := gocontext.Background()
	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.StringVariationCtx(ctx, flagKey, context, "default")

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	assert.Len(t, exportedSpans, 1)
	events := exportedSpans[0].Events()
	assert.Len(t, events, 1)
	flagEvent := events[0]
	assert.Equal(t, "feature_flag", flagEvent.Name)

	attributes := attribute.NewSet(flagEvent.Attributes...)
	attributeVariationIndex, _ := (&attributes).Value("feature_flag.result.variationIndex")
	assert.Equal(t, int64(2), attributeVariationIndex.AsInt64())
}

func TestEnvironmentIDIsOptional(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing()
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	events := exportedSpans[0].Events()
	flagEvent := events[0]

	attributes := attribute.NewSet(flagEvent.Attributes...)
	_, attributeSetIDPresent := (&attributes).Value("feature_flag.set.id")
	assert.False(t, attributeSetIDPresent)
}

func TestEnvironmentIDFromHookOptions(t *testing.T) {
	exporter := configureMemoryExporter()
	tracer := otel.Tracer("launchdarkly-client")
	client, _ := createClientWithTracing(WithEnvironmentID("env-id-from-options"))
	context := ldcontext.New("test-context")

	ctx := gocontext.Background()

	ctx, span := tracer.Start(ctx, spanName)

	_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

	span.End()

	exportedSpans := exporter.GetSpans().Snapshots()
	events := exportedSpans[0].Events()
	flagEvent := events[0]

	attributes := attribute.NewSet(flagEvent.Attributes...)
	attributeSetID, _ := (&attributes).Value("feature_flag.set.id")
	assert.Equal(t, "env-id-from-options", attributeSetID.AsString())
}

func TestEnvironmentIDFromSeriesContext(t *testing.T) {
	streamHandler := createStreamHandlerWithEnvironmentID("env-id-from-context")
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		exporter := configureMemoryExporter()
		tracer := otel.Tracer("launchdarkly-client")
		client := createClientWithStreamServerAndTracing(streamServer)
		defer client.Close()
		context := ldcontext.New("test-context")

		ctx := gocontext.Background()

		ctx, span := tracer.Start(ctx, spanName)

		_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

		span.End()

		exportedSpans := exporter.GetSpans().Snapshots()
		events := exportedSpans[0].Events()
		flagEvent := events[0]

		attributes := attribute.NewSet(flagEvent.Attributes...)
		attributeSetID, _ := (&attributes).Value("feature_flag.set.id")
		assert.Equal(t, "env-id-from-context", attributeSetID.AsString())
	})
}

func TestEnvironmentIDFromHookOptionsOverridesSeriesContext(t *testing.T) {
	streamHandler := createStreamHandlerWithEnvironmentID("env-id-from-context")
	httphelpers.WithServer(streamHandler, func(streamServer *httptest.Server) {
		exporter := configureMemoryExporter()
		tracer := otel.Tracer("launchdarkly-client")
		client := createClientWithStreamServerAndTracing(streamServer, WithEnvironmentID("env-id-from-options"))
		defer client.Close()
		context := ldcontext.New("test-context")

		ctx := gocontext.Background()

		ctx, span := tracer.Start(ctx, spanName)

		_, _ = client.BoolVariationCtx(ctx, flagKey, context, false)

		span.End()

		exportedSpans := exporter.GetSpans().Snapshots()
		events := exportedSpans[0].Events()
		flagEvent := events[0]

		attributes := attribute.NewSet(flagEvent.Attributes...)
		attributeSetID, _ := (&attributes).Value("feature_flag.set.id")
		assert.Equal(t, "env-id-from-options", attributeSetID.AsString())
	})
}
