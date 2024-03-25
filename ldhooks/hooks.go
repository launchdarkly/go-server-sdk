package ldhooks

import (
	"context"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
)

// Implementation Note: The UnimplementedHook struct is provided to simplify hook implementation. It should always
// contain an implementation of all series and handler interfaces. It should not contain the Hook interface directly
// because the implementer should be required to implement GetMetadata.

// A Hook is used to extend the functionality of the SDK.
//
// In order to avoid implementing unused methods, as well as easing maintenance of compatibility, implementors should
// compose the `UnimplementedHook`.
//
//	type MyHook struct {
//	  UnimplementedHook
//	}
type Hook interface {
	GetMetadata() HookMetadata
	EvaluationSeries
}

// The EvaluationSeries is composed of stages, methods that are called during the evaluation of flags.
type EvaluationSeries interface {
	// BeforeEvaluation is called during the execution of a variation Method before the flag value has been determined.
	// The Method returns EvaluationSeriesData that will be passed to the next stage in the evaluation
	// series.
	//
	// The EvaluationSeriesData returned should always contain the previous data as well as any new data which is
	// required for subsequent stage execution.
	BeforeEvaluation(
		ctx context.Context,
		seriesContext EvaluationSeriesContext,
		data EvaluationSeriesData,
	) (EvaluationSeriesData, error)

	// AfterEvaluation is called during the execution of the variation Method after the flag value has been determined.
	// The Method returns EvaluationSeriesData that will be passed to the next stage in the evaluation
	// series.
	//
	// The EvaluationSeriesData returned should always contain the previous data as well as any new data which is
	// required for subsequent stage execution.
	AfterEvaluation(ctx context.Context,
		seriesContext EvaluationSeriesContext,
		data EvaluationSeriesData,
		detail ldreason.EvaluationDetail,
	) (EvaluationSeriesData, error)
}

// hookInterfaces is an interface for implementation by the UnimplementedHook
type hookInterfaces interface {
	EvaluationSeries
}

// UnimplementedHook implements all Hook methods with empty functions.
// Hook implementors should use this to avoid having to implement empty methods and to ease updates when the Hook
// interface is extended.
//
//	type MyHook struct {
//	  UnimplementedHook
//	}
//
// The hook should implement at least one stage/handler as well as the GetMetadata function.
type UnimplementedHook struct {
}

// BeforeEvaluation is a default implementation of the BeforeEvaluation stage.
func (h UnimplementedHook) BeforeEvaluation(
	_ context.Context,
	_ EvaluationSeriesContext,
	data EvaluationSeriesData,
) (EvaluationSeriesData, error) {
	return data, nil
}

// AfterEvaluation is a default implementation of the AfterEvaluation stage.
func (h UnimplementedHook) AfterEvaluation(
	_ context.Context,
	_ EvaluationSeriesContext,
	data EvaluationSeriesData,
	_ ldreason.EvaluationDetail,
) (EvaluationSeriesData, error) {
	return data, nil
}

// Implementation note: UnimplementedHook does not implement GetMetaData because that must be implemented by hook
// implementors.

// Ensure UnimplementedHook implements required interfaces.
var _ hookInterfaces = UnimplementedHook{}
