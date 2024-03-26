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
	defaultValue  any
	context       gocontext.Context
}

const hookTestFlag string = "test-flag"

func TestHooksAreExecutedForAllVariationMethods(t *testing.T) {
	testContext := ldcontext.New("test-context")
	testGoContext := gocontext.WithValue(gocontext.TODO(), "test-key", "test-value")

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
			defaultValue: false,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.BoolVariationEx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationEx",
			defaultValue: false,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.BoolVariationDetail(hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationDetail",
			defaultValue: false,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.BoolVariationDetailEx(testGoContext, hookTestFlag, testContext, false)
			},
			methodName:   "LDClient.BoolVariationDetailEx",
			defaultValue: false,
			context:      testGoContext,
		},
		// Int variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.IntVariation(hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariation",
			defaultValue: 42,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.IntVariationEx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationEx",
			defaultValue: 42,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.IntVariationDetail(hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationDetail",
			defaultValue: 42,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.IntVariationDetailEx(testGoContext, hookTestFlag, testContext, 42)
			},
			methodName:   "LDClient.IntVariationDetailEx",
			defaultValue: 42,
			context:      testGoContext,
		},
		// Float variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.Float64Variation(hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64Variation",
			defaultValue: 3.14,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.Float64VariationEx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationEx",
			defaultValue: 3.14,
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.Float64VariationDetail(hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationDetail",
			defaultValue: 3.14,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.Float64VariationDetailEx(testGoContext, hookTestFlag, testContext, 3.14)
			},
			methodName:   "LDClient.Float64VariationDetailEx",
			defaultValue: 3.14,
			context:      testGoContext,
		},
		// String variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.StringVariation(hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariation",
			defaultValue: "test-string",
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.StringVariationEx(testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationEx",
			defaultValue: "test-string",
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.StringVariationDetail(hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationDetail",
			defaultValue: "test-string",
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.StringVariationDetailEx(
					testGoContext, hookTestFlag, testContext, "test-string")
			},
			methodName:   "LDClient.StringVariationDetailEx",
			defaultValue: "test-string",
			context:      testGoContext,
		},
		// JSON variations
		{
			variationCall: func(client *LDClient) {
				_, _ = client.JSONVariation(hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariation",
			defaultValue: "test-string",
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _ = client.JSONVariationEx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationEx",
			defaultValue: "test-string",
			context:      testGoContext,
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.JSONVariationDetail(hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationDetail",
			defaultValue: "test-string",
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.JSONVariationDetailEx(
					testGoContext, hookTestFlag, testContext, ldvalue.String("test-string"))
			},
			methodName:   "LDClient.JSONVariationDetailEx",
			defaultValue: "test-string",
			context:      testGoContext,
		},
		// Migration variation
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.MigrationVariation(hookTestFlag, testContext, ldmigration.Off)
			},
			methodName:   "LDClient.MigrationVariation",
			defaultValue: ldmigration.Off,
			context:      gocontext.TODO(),
		},
		{
			variationCall: func(client *LDClient) {
				_, _, _ = client.MigrationVariationEx(testGoContext, hookTestFlag, testContext, ldmigration.Off)
			},
			methodName:   "LDClient.MigrationVariationEx",
			defaultValue: ldmigration.Off,
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
