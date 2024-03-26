package sharedtest

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// HookStage is the stage of a hook being executed.
type HookStage string

const (
	// HookStageBeforeEvaluation is the stage executed before evaluation.
	HookStageBeforeEvaluation HookStage = "before"
	// HookStageAfterEvaluation is the stage executed after evaluation.
	HookStageAfterEvaluation = "after"
)

// HookEvalCapture is used to capture the information provided to a hook during execution.
type HookEvalCapture struct {
	GoContext               context.Context
	EvaluationSeriesContext ldhooks.EvaluationSeriesContext
	EvaluationSeriesData    ldhooks.EvaluationSeriesData
	Detail                  ldreason.EvaluationDetail
}

// HookExpectedCall represents an expected call to a hook.
type HookExpectedCall struct {
	HookStage   HookStage
	EvalCapture HookEvalCapture
}

type hookTestData struct {
	captureBefore []HookEvalCapture
	captureAfter  []HookEvalCapture
}

// TestHook is a hook for testing to be used only by the SDK tests.
type TestHook struct {
	testData     *hookTestData
	metadata     ldhooks.HookMetadata
	BeforeInject func(context.Context, ldhooks.EvaluationSeriesContext,
		ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error)

	AfterInject func(context.Context, ldhooks.EvaluationSeriesContext,
		ldhooks.EvaluationSeriesData, ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error)
}

// NewTestHook creates a new test hook.
func NewTestHook(name string) TestHook {
	return TestHook{
		testData: &hookTestData{
			captureBefore: make([]HookEvalCapture, 0),
			captureAfter:  make([]HookEvalCapture, 0),
		},
		BeforeInject: func(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
			data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return data, nil
		},
		AfterInject: func(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
			data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error) {
			return data, nil
		},
		metadata: ldhooks.NewHookMetadata(name),
	}
}

// GetMetadata gets the meta-data for the hook.
func (h TestHook) GetMetadata() ldhooks.HookMetadata {
	return h.metadata
}

// BeforeEvaluation testing implementation of the BeforeEvaluation stage.
func (h TestHook) BeforeEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.captureBefore = append(h.testData.captureBefore, HookEvalCapture{
		EvaluationSeriesContext: seriesContext,
		EvaluationSeriesData:    data,
		GoContext:               ctx,
	})
	return h.BeforeInject(ctx, seriesContext, data)
}

// AfterEvaluation testing implementation of the AfterEvaluation stage.
func (h TestHook) AfterEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
	detail ldreason.EvaluationDetail,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.captureAfter = append(h.testData.captureAfter, HookEvalCapture{
		EvaluationSeriesContext: seriesContext,
		EvaluationSeriesData:    data,
		Detail:                  detail,
		GoContext:               ctx,
	})
	return h.AfterInject(ctx, seriesContext, data, detail)
}

// Verify is used to verify that the hook received calls it expected.
func (h TestHook) Verify(t *testing.T, calls ...HookExpectedCall) {
	localBeforeCalls := make([]HookEvalCapture, len(h.testData.captureBefore))
	localAfterCalls := make([]HookEvalCapture, len(h.testData.captureAfter))

	copy(localBeforeCalls, h.testData.captureBefore)
	copy(localAfterCalls, h.testData.captureAfter)

	for _, call := range calls {
		found := false
		switch call.HookStage {
		case HookStageBeforeEvaluation:
			for i, beforeCall := range localBeforeCalls {
				if reflect.DeepEqual(beforeCall, call.EvalCapture) {
					localBeforeCalls = slices.Delete(localBeforeCalls, i, i+1)
					found = true
				} else {
					logDebugData(t, beforeCall, call)
				}
			}
		case HookStageAfterEvaluation:
			for i, afterCall := range localAfterCalls {
				if reflect.DeepEqual(afterCall, call.EvalCapture) {
					localAfterCalls = slices.Delete(localAfterCalls, i, i+1)
					found = true
				} else {
					logDebugData(t, afterCall, call)
				}
			}
		default:
			assert.FailNow(t, fmt.Sprintf("Unhandled hook stage: %v", call.HookStage))
		}
		if !found {
			assert.FailNow(t, fmt.Sprintf("Unable to find matching call: %+v", call))
		}
	}
}

func (h TestHook) VerifyNoCalls(t *testing.T) {
	assert.Empty(t, h.testData.captureBefore)
	assert.Empty(t, h.testData.captureAfter)
}

func logDebugData(t *testing.T, afterCall HookEvalCapture, call HookExpectedCall) {
	// Log some information to help understand test failures.
	if !reflect.DeepEqual(afterCall.GoContext, call.EvalCapture.GoContext) {
		t.Log("Go context not equal")
	}
	if !reflect.DeepEqual(afterCall.Detail, call.EvalCapture.Detail) {
		t.Log("Evaluation detail not equal")
	}
	if !reflect.DeepEqual(afterCall.EvaluationSeriesData, call.EvalCapture.EvaluationSeriesData) {
		t.Log("Evaluation series data not equal")
	}
	if !reflect.DeepEqual(afterCall.EvaluationSeriesContext, call.EvalCapture.EvaluationSeriesContext) {
		t.Log("Evaluation series context not equal")
	}
}
