package ldclient

import (
	gocontext "context"
	"fmt"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// The execution of hooks is mostly tested in hook_runner_test. The tests here are to test the usage of the hook runner
// by the client, but not the implementation of the hook runner itself.

type parameterizedVariation struct {
	variationCall func(client *LDClient)
	methodName    string
	defaultValue  ldvalue.Value
	context       gocontext.Context
}

const hookTestFlag string = "test-flag"

func TestHooksAreExecutedForAllVariationMethods(t *testing.T) {
	testContext := ldcontext.New("test-context")
	testGoContext := gocontext.WithValue(gocontext.TODO(), "test-key", "test-value")
	falseValue := ldvalue.Bool(false)
	fortyTwoValue := ldvalue.Int(42)
	piValue := ldvalue.Float64(3.14)
	stringValue := ldvalue.String("test-string")

	beforeCapture := func(testCase parameterizedVariation) sharedtest.HookExpectedCall {
		return sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageBeforeEvaluation,
			EvalCapture: sharedtest.HookEvalCapture{
				EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext("test-flag", testContext,
					testCase.defaultValue, testCase.methodName),
				EvaluationSeriesData: ldhooks.EmptyEvaluationSeriesData(),
				GoContext:            testCase.context,
			}}
	}

	afterCapture := func(testCase parameterizedVariation) sharedtest.HookExpectedCall {
		return sharedtest.HookExpectedCall{
			HookStage: sharedtest.HookStageAfterEvaluation,
			EvalCapture: sharedtest.HookEvalCapture{
				EvaluationSeriesContext: ldhooks.NewEvaluationSeriesContext(hookTestFlag, testContext,
					testCase.defaultValue, testCase.methodName),
				EvaluationSeriesData: ldhooks.NewEvaluationSeriesBuilder(ldhooks.EmptyEvaluationSeriesData()).
					Set("test-key", "test-value").Build(),
				Detail: ldreason.NewEvaluationDetailForError(
					ldreason.EvalErrorClientNotReady,
					ldvalue.CopyArbitraryValue(testCase.defaultValue),
				),
				GoContext: testCase.context,
			}}
	}

	testCases := []parameterizedVariation{
		// Bool variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.BoolVariation(hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariation",
			defaultValue: falseValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.BoolVariationCtx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationCtx",
			defaultValue: falseValue,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.BoolVariationDetail(hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationDetail",
			defaultValue: falseValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.BoolVariationDetailCtx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationDetailCtx",
			defaultValue: falseValue,
			context:      testGoContext,
		},
		// Int variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.IntVariation(hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariation",
			defaultValue: fortyTwoValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.IntVariationCtx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationCtx",
			defaultValue: fortyTwoValue,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.IntVariationDetail(hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationDetail",
			defaultValue: fortyTwoValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.IntVariationDetailCtx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationDetailCtx",
			defaultValue: fortyTwoValue,
			context:      testGoContext,
		},
		// Float variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.Float64Variation(hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64Variation",
			defaultValue: piValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.Float64VariationCtx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationCtx",
			defaultValue: piValue,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.Float64VariationDetail(hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationDetail",
			defaultValue: piValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.Float64VariationDetailCtx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationDetailCtx",
			defaultValue: piValue,
			context:      testGoContext,
		},
		// String variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.StringVariation(hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariation",
			defaultValue: stringValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.StringVariationCtx(testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationCtx",
			defaultValue: stringValue,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.StringVariationDetail(hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationDetail",
			defaultValue: stringValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.StringVariationDetailCtx(
					testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationDetailCtx",
			defaultValue: stringValue,
			context:      testGoContext,
		},
		// JSON variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.JSONVariation(hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariation",
			defaultValue: stringValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.JSONVariationCtx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationCtx",
			defaultValue: stringValue,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.JSONVariationDetail(hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationDetail",
			defaultValue: stringValue,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.JSONVariationDetailCtx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationDetailCtx",
			defaultValue: stringValue,
			context:      testGoContext,
		},
		// Migration variation
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.MigrationVariation(hookTestFlag, testContext, ldmigration.Off)
			},
			methodName:   "LDClient.MigrationVariation",
			defaultValue: ldvalue.String(string(ldmigration.Off)),
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.MigrationVariationCtx(testGoContext, hookTestFlag, testContext, ldmigration.Off)
			},
			methodName:   "LDClient.MigrationVariationCtx",
			defaultValue: ldvalue.String(string(ldmigration.Off)),
			context:      testGoContext,
		},
	}

	for _, testCase := range testCases {
		t.Run(fmt.Sprintf("for method %v", testCase.methodName), func(t *testing.T) {
			hook := sharedtest.NewTestHook("test-hook")
			hook.BeforeInject = func(
				ctx gocontext.Context,
				context ldhooks.EvaluationSeriesContext,
				data ldhooks.EvaluationSeriesData,
			) (ldhooks.EvaluationSeriesData, error) {
				return ldhooks.NewEvaluationSeriesBuilder(data).Set("test-key", "test-value").Build(), nil
			}
			client, _ := MakeCustomClient("", Config{Offline: true, Hooks: []ldhooks.Hook{hook}}, 0)
			testCase.variationCall(client)
			hook.Verify(
				t,
				beforeCapture(testCase),
				afterCapture(testCase),
			)
		})
	}
}
