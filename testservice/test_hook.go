package main

import (
	"context"
	"errors"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/testservice/servicedef"
)

type testHook struct {
	ldhooks.Unimplemented
	metadata        ldhooks.Metadata
	dataPayloads    map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData
	errors          map[servicedef.HookStage]string
	callbackService callbackService
}

func newTestHook(
	name string,
	endpoint string,
	data map[servicedef.HookStage]servicedef.SDKConfigEvaluationHookData,
	errors map[servicedef.HookStage]string,
) testHook {
	return testHook{
		metadata:        ldhooks.NewMetadata(name),
		dataPayloads:    data,
		callbackService: callbackService{baseURL: endpoint},
		errors:          errors,
	}
}

func (t testHook) Metadata() ldhooks.Metadata {
	return t.metadata
}

func (t testHook) BeforeEvaluation(
	_ context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
) (ldhooks.EvaluationSeriesData, error) {
	errString, hasErr := t.errors[servicedef.BeforeEvaluation]
	if hasErr {
		return data, errors.New(errString)
	}
	err := t.callbackService.post("", servicedef.HookExecutionEvaluationPayload{
		EvaluationSeriesContext: evaluationSeriesContextToService(seriesContext),
		EvaluationSeriesData:    evaluationSeriesDataToService(data),
		Stage:                   servicedef.BeforeEvaluation,
	}, nil)
	if err != nil {
		return ldhooks.EmptyEvaluationSeriesData(), err
	}
	dataBuilder := ldhooks.NewEvaluationSeriesBuilder(data)
	stageData := t.dataPayloads[servicedef.BeforeEvaluation]
	for key, value := range stageData {
		dataBuilder.Set(key, value)
	}
	return dataBuilder.Build(), nil
}

func (t testHook) AfterEvaluation(
	_ context.Context,
	seriesContext ldhooks.EvaluationSeriesContext,
	data ldhooks.EvaluationSeriesData,
	detail ldreason.EvaluationDetail,
) (ldhooks.EvaluationSeriesData, error) {
	errString, hasErr := t.errors[servicedef.AfterEvaluation]
	if hasErr {
		return data, errors.New(errString)
	}
	err := t.callbackService.post("", servicedef.HookExecutionEvaluationPayload{
		EvaluationSeriesContext: evaluationSeriesContextToService(seriesContext),
		EvaluationSeriesData:    evaluationSeriesDataToService(data),
		Stage:                   servicedef.AfterEvaluation,
		EvaluationDetail:        *detailToService(detail),
	}, nil)
	if err != nil {
		return ldhooks.EmptyEvaluationSeriesData(), err
	}
	dataBuilder := ldhooks.NewEvaluationSeriesBuilder(data)
	stageData := t.dataPayloads[servicedef.AfterEvaluation]
	for key, value := range stageData {
		dataBuilder.Set(key, value)
	}
	return data, nil
}

func (t testHook) AfterTrack(
	_ context.Context,
	seriesContext ldhooks.TrackSeriesContext,
) error {
	errString, hasErr := t.errors[servicedef.AfterTrack]
	if hasErr {
		return errors.New(errString)
	}
	return t.callbackService.post("", servicedef.HookExecutionTrackPayload{
		TrackSeriesContext: trackSeriesContextToService(seriesContext),
		Stage:              servicedef.AfterTrack,
	}, nil)
}

func evaluationSeriesContextToService(
	seriesContext ldhooks.EvaluationSeriesContext,
) servicedef.EvaluationSeriesContext {
	return servicedef.EvaluationSeriesContext{
		FlagKey:       seriesContext.FlagKey(),
		Context:       seriesContext.Context(),
		DefaultValue:  seriesContext.DefaultValue(),
		Method:        seriesContext.Method(),
		EnvironmentID: seriesContext.EnvironmentID(),
	}
}

func evaluationSeriesDataToService(seriesData ldhooks.EvaluationSeriesData) map[string]ldvalue.Value {
	ret := make(map[string]ldvalue.Value)
	for key, value := range seriesData.AsAnyMap() {
		ret[key] = ldvalue.CopyArbitraryValue(value)
	}
	return ret
}

func detailToService(detail ldreason.EvaluationDetail) *servicedef.EvaluateFlagResponse {
	rep := &servicedef.EvaluateFlagResponse{
		Value:          detail.Value,
		VariationIndex: detail.VariationIndex.AsPointer(),
	}
	if detail.Reason.IsDefined() {
		rep.Reason = &detail.Reason
	}
	return rep
}

func trackSeriesContextToService(
	seriesContext ldhooks.TrackSeriesContext,
) servicedef.TrackSeriesContext {
	return servicedef.TrackSeriesContext{
		Key:           seriesContext.Key(),
		Context:       seriesContext.Context(),
		MetricValue:   seriesContext.MetricValue(),
		Data:          seriesContext.Data(),
		EnvironmentID: seriesContext.EnvironmentID(),
	}
}
