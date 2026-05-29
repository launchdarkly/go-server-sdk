package sharedtest

import (
	"context"
	"reflect"
	"testing"

	"slices"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-sdk-common/v4/ldreason"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
)

// HookStage is the stage of a hook being executed.
type HookStage string

const (
	// HookStageBeforeEvaluation is the stage executed before evaluation.
	HookStageBeforeEvaluation = HookStage("beforeEvaluation")
	// HookStageAfterEvaluation is the stage executed after evaluation.
	HookStageAfterEvaluation = HookStage("afterEvaluation")
	// HookStageAfterTrack is the stage executed after track.
	HookStageAfterTrack = HookStage("afterTrack")
)

// HookEvalCapture is used to capture the information provided to a hook during execution.
type HookEvalCapture struct {
	GoContext               context.Context
	EvaluationSeriesContext ldhooks.EvaluationSeriesContext
	EvaluationSeriesData    ldhooks.EvaluationSeriesData
	EvaluationDetail        ldreason.EvaluationDetail
}

// HookTrackCapture is used to capture the information provided to a hook during execution.
type HookTrackCapture struct {
	GoContext          context.Context
	TrackSeriesContext ldhooks.TrackSeriesContext
}

// HookExpectedCall represents an expected call to a hook.
type HookExpectedCall struct {
	HookStage    HookStage
	EvalCapture  HookEvalCapture
	TrackCapture HookTrackCapture
}

type hookTestData struct {
	captureBeforeEval []HookEvalCapture
	captureAfterEval  []HookEvalCapture
	captureAfterTrack []HookTrackCapture
}

// TestHook is a hook for testing to be used only by the SDK tests.
type TestHook struct {
	ldhooks.Unimplemented
	testData               *hookTestData
	metadata               ldhooks.Metadata
	BeforeEvaluationInject func(context.Context, ldhooks.EvaluationSeriesContext,
		ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error)

	AfterEvaluationInject func(context.Context, ldhooks.EvaluationSeriesContext,
		ldhooks.EvaluationSeriesData, ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error)

	AfterTrackInject func(context.Context, ldhooks.TrackSeriesContext) error
}

// NewTestHook creates a new test hook.
func NewTestHook(name string) TestHook {
	return TestHook{
		testData: &hookTestData{
			captureBeforeEval: make([]HookEvalCapture, 0),
			captureAfterEval:  make([]HookEvalCapture, 0),
			captureAfterTrack: make([]HookTrackCapture, 0),
		},
		BeforeEvaluationInject: func(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
			data ldhooks.EvaluationSeriesData) (ldhooks.EvaluationSeriesData, error) {
			return data, nil
		},
		AfterEvaluationInject: func(ctx context.Context, seriesContext ldhooks.EvaluationSeriesContext,
			data ldhooks.EvaluationSeriesData, detail ldreason.EvaluationDetail) (ldhooks.EvaluationSeriesData, error) {
			return data, nil
		},
		AfterTrackInject: func(ctx context.Context, seriesContext ldhooks.TrackSeriesContext) error {
			return nil
		},
		metadata: ldhooks.NewMetadata(name),
	}
}

// Metadata gets the meta-data for the hook.
func (h TestHook) Metadata() ldhooks.Metadata {
	return h.metadata
}

// BeforeEvaluation testing implementation of the BeforeEvaluation stage.
func (h TestHook) BeforeEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.captureBeforeEval = append(h.testData.captureBeforeEval, HookEvalCapture{
		EvaluationSeriesContext: seriesContext,
		EvaluationSeriesData:    data,
		GoContext:               ctx,
	})
	return h.BeforeEvaluationInject(ctx, seriesContext, data)
}

// AfterEvaluation testing implementation of the AfterEvaluation stage.
func (h TestHook) AfterEvaluation(
	ctx context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
	detail ldreason.EvaluationDetail,
) (ldhooks.EvaluationSeriesData, error) {
	h.testData.captureAfterEval = append(h.testData.captureAfterEval, HookEvalCapture{
		EvaluationSeriesContext: seriesContext,
		EvaluationSeriesData:    data,
		EvaluationDetail:        detail,
		GoContext:               ctx,
	})
	return h.AfterEvaluationInject(ctx, seriesContext, data, detail)
}

// AfterTrack testing implementation of the AfterTrack stage.
func (h TestHook) AfterTrack(ctx context.Context, seriesContext ldhooks.TrackSeriesContext) error {
	h.testData.captureAfterTrack = append(h.testData.captureAfterTrack, HookTrackCapture{
		GoContext:          ctx,
		TrackSeriesContext: seriesContext,
	})
	return h.AfterTrackInject(ctx, seriesContext)
}

// Verify is used to verify that the hook received calls it expected.
func (h TestHook) Verify(t *testing.T, calls ...HookExpectedCall) {
	localBeforeEvalCalls := make([]HookEvalCapture, len(h.testData.captureBeforeEval))
	localAfterEvalCalls := make([]HookEvalCapture, len(h.testData.captureAfterEval))
	localAfterTrackCalls := make([]HookTrackCapture, len(h.testData.captureAfterTrack))

	copy(localBeforeEvalCalls, h.testData.captureBeforeEval)
	copy(localAfterEvalCalls, h.testData.captureAfterEval)
	copy(localAfterTrackCalls, h.testData.captureAfterTrack)

	for _, call := range calls {
		found := false
		switch call.HookStage {
		case HookStageBeforeEvaluation:
			for i, beforeCall := range localBeforeEvalCalls {
				if reflect.DeepEqual(beforeCall, call.EvalCapture) {
					localBeforeEvalCalls = slices.Delete(localBeforeEvalCalls, i, i+1)
					found = true
				} else {
					logDebugEvalData(t, beforeCall, call)
				}
			}
		case HookStageAfterEvaluation:
			for i, afterCall := range localAfterEvalCalls {
				if reflect.DeepEqual(afterCall, call.EvalCapture) {
					localAfterEvalCalls = slices.Delete(localAfterEvalCalls, i, i+1)
					found = true
				} else {
					logDebugEvalData(t, afterCall, call)
				}
			}
		case HookStageAfterTrack:
			for i, afterCall := range localAfterTrackCalls {
				if reflect.DeepEqual(afterCall, call.TrackCapture) {
					localAfterTrackCalls = slices.Delete(localAfterTrackCalls, i, i+1)
					found = true
				} else {
					logDebugTrackData(t, afterCall, call)
				}
			}
		default:
			assert.FailNowf(t, "Unhandled hook stage", "stage: %v", call.HookStage)
		}
		if !found {
			assert.FailNowf(t, "Unable to find matching call", "details: %+v", call)
		}
	}
}

// VerifyNoCalls will assert if the hook has received any calls.
func (h TestHook) VerifyNoCalls(t *testing.T) {
	assert.Empty(t, h.testData.captureBeforeEval)
	assert.Empty(t, h.testData.captureAfterEval)
	assert.Empty(t, h.testData.captureAfterTrack)
}

func logDebugEvalData(t *testing.T, afterCall HookEvalCapture, call HookExpectedCall) {
	// Log some information to help understand test failures.
	if !reflect.DeepEqual(afterCall.GoContext, call.EvalCapture.GoContext) {
		t.Log("Go context not equal")
	}
	if !reflect.DeepEqual(afterCall.EvaluationDetail, call.EvalCapture.EvaluationDetail) {
		t.Log("Evaluation detail not equal")
	}
	if !reflect.DeepEqual(afterCall.EvaluationSeriesData, call.EvalCapture.EvaluationSeriesData) {
		t.Log("Evaluation series data not equal")
	}
	if !reflect.DeepEqual(afterCall.EvaluationSeriesContext, call.EvalCapture.EvaluationSeriesContext) {
		t.Log("Evaluation series context not equal")
	}
}

func logDebugTrackData(t *testing.T, afterCall HookTrackCapture, call HookExpectedCall) {
	// Log some information to help understand test failures.
	if !reflect.DeepEqual(afterCall.GoContext, call.EvalCapture.GoContext) {
		t.Log("Go context not equal")
	}
	if !reflect.DeepEqual(afterCall.TrackSeriesContext, call.TrackCapture.TrackSeriesContext) {
		t.Log("Track series context not equal")
	}
}
