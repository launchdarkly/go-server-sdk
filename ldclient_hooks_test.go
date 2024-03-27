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
	stringValue := ldvalue.String("string-value")

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
				_, _ = client.BoolVariationEx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationEx",
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
				_, _, _ = client.BoolVariationDetailEx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationDetailEx",
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
				_, _ = client.IntVariationEx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationEx",
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
				_, _, _ = client.IntVariationDetailEx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationDetailEx",
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
				_, _ = client.Float64VariationEx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationEx",
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
				_, _, _ = client.Float64VariationDetailEx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationDetailEx",
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
				_, _ = client.StringVariationEx(testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationEx",
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
				_, _, _ = client.StringVariationDetailEx(
					testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationDetailEx",
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
				_, _ = client.JSONVariationEx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationEx",
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
				_, _, _ = client.JSONVariationDetailEx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationDetailEx",
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
				_, _, _ = client.MigrationVariationEx(testGoContext, hookTestFlag, testContext, ldmigration.Off)
			},
			methodName:   "LDClient.MigrationVariationEx",
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

func TestUsesAStableSetOfHooksDuringEvaluation(t *testing.T) {
	client, _ := MakeCustomClient("", Config{Offline: true, Hooks: []ldhooks.Hook{}}, 0)

	hook := sharedtest.NewTestHook("test-hook")
	sneaky := sharedtest.NewTestHook("sneaky")
	hook.BeforeInject = func(
		ctx gocontext.Context,
		context ldhooks.EvaluationSeriesContext,
		data ldhooks.EvaluationSeriesData,
	) (ldhooks.EvaluationSeriesData, error) {
		client.AddHooks(sneaky)
		return ldhooks.NewEvaluationSeriesBuilder(data).Set("test-key", "test-value").Build(), nil
	}

	client.AddHooks(hook)

	_, _ = client.BoolVariation("flag-key", ldcontext.New("test-context"), false)

	sneaky.VerifyNoCalls(t)
}
