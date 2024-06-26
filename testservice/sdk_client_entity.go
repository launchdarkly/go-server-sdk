package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	ld "github.com/launchdarkly/go-server-sdk/v7"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldhooks"
	"github.com/launchdarkly/go-server-sdk/v7/testservice/servicedef"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
)

const defaultStartWaitTime = 5 * time.Second

type SDKClientEntity struct {
	sdk    *ld.LDClient
	logger *log.Logger
}

func NewSDKClientEntity(params servicedef.CreateInstanceParams) (*SDKClientEntity, error) {
	c := &SDKClientEntity{}
	c.logger = log.New(os.Stdout, fmt.Sprintf("[%s]: ", params.Tag),
		log.Ldate|log.Ltime|log.Lmicroseconds|log.Lmsgprefix)
	c.logger.Printf("Starting SDK client with configuration: %s", asJSON(params))

	sdkLog := ldlog.NewDefaultLoggers()
	sdkLog.SetBaseLogger(c.logger)
	sdkLog.SetPrefix("[sdklog]")
	sdkLog.SetMinLevel(ldlog.Debug)

	ldConfig := makeSDKConfig(params.Configuration, sdkLog)

	startWaitTime := defaultStartWaitTime
	if params.Configuration.StartWaitTimeMS > 0 {
		startWaitTime = time.Millisecond * time.Duration(params.Configuration.StartWaitTimeMS)
	}
	sdk, err := ld.MakeCustomClient(params.Configuration.Credential, ldConfig, startWaitTime)
	if sdk == nil || (err != nil && !params.Configuration.InitCanFail) {
		if sdk != nil {
			_ = sdk.Close()
		}
		return nil, err
	}
	c.sdk = sdk

	return c, nil
}

func (c *SDKClientEntity) Close() {
	_ = c.sdk.Close()
	c.logger.Println("Test ended")
	c.logger.SetOutput(io.Discard)
}

func contextOrUser(context ldcontext.Context, maybeUser *ldcontext.Context) ldcontext.Context {
	if maybeUser != nil {
		return *maybeUser
	}
	return context
}

func (c *SDKClientEntity) DoCommand(params servicedef.CommandParams) (interface{}, error) {
	c.logger.Printf("Test service sent command: %s", asJSON(params))
	switch params.Command {
	case servicedef.CommandEvaluateFlag:
		return c.evaluateFlag(*params.Evaluate)
	case servicedef.CommandEvaluateAllFlags:
		return c.evaluateAllFlags(*params.EvaluateAll)
	case servicedef.CommandIdentifyEvent:
		err := c.sdk.Identify(params.IdentifyEvent.Context)
		return nil, err
	case servicedef.CommandCustomEvent:
		context := params.CustomEvent.Context
		if params.CustomEvent.MetricValue != nil {
			return nil, c.sdk.TrackMetric(params.CustomEvent.EventKey, context,
				*params.CustomEvent.MetricValue, params.CustomEvent.Data)
		}
		if params.CustomEvent.Data.IsDefined() {
			return nil, c.sdk.TrackData(params.CustomEvent.EventKey, context, params.CustomEvent.Data)
		}
		return nil, c.sdk.TrackEvent(params.CustomEvent.EventKey, context)
	case servicedef.CommandFlushEvents:
		c.sdk.Flush()
		return nil, nil
	case servicedef.CommandGetBigSegmentStoreStatus:
		bigSegmentsStatus := c.sdk.GetBigSegmentStoreStatusProvider().GetStatus()
		return servicedef.BigSegmentStoreStatusResponse(bigSegmentsStatus), nil
	case servicedef.CommandContextBuild:
		return c.contextBuild(*params.ContextBuild)
	case servicedef.CommandContextConvert:
		return c.contextConvert(*params.ContextConvert)
	case servicedef.CommandSecureModeHash:
		hash := c.sdk.SecureModeHash(params.SecureModeHash.Context)
		return servicedef.SecureModeHashResponse{Result: hash}, nil
	case servicedef.CommandMigrationVariation:
		// Normal operation should inspect this err. However, the test harness
		// will not generate invalid variation configurations for this test.
		// More complex error conditions are tested through the migration
		// operation command.
		stage, _, _ := c.sdk.MigrationVariation(params.MigrationVariation.Key, params.MigrationVariation.Context, params.MigrationVariation.DefaultStage)
		return servicedef.MigrationVariationResponse{Result: string(stage)}, nil
	case servicedef.CommandMigrationOperation:
		return c.migrationOperation(*params.MigrationOperation)
	default:
		return nil, BadRequestError{Message: fmt.Sprintf("unknown command %q", params.Command)}
	}
}

func (c *SDKClientEntity) evaluateFlag(p servicedef.EvaluateFlagParams) (*servicedef.EvaluateFlagResponse, error) {
	context := p.Context
	var result ldreason.EvaluationDetail
	if p.Detail {
		switch p.ValueType {
		case servicedef.ValueTypeBool:
			var boolValue bool
			boolValue, result, _ = c.sdk.BoolVariationDetail(p.FlagKey, context, p.DefaultValue.BoolValue())
			result.Value = ldvalue.Bool(boolValue)
		case servicedef.ValueTypeInt:
			var intValue int
			intValue, result, _ = c.sdk.IntVariationDetail(p.FlagKey, context, p.DefaultValue.IntValue())
			result.Value = ldvalue.Int(intValue)
		case servicedef.ValueTypeDouble:
			var floatValue float64
			floatValue, result, _ = c.sdk.Float64VariationDetail(p.FlagKey, context, p.DefaultValue.Float64Value())
			result.Value = ldvalue.Float64(floatValue)
		case servicedef.ValueTypeString:
			var strValue string
			strValue, result, _ = c.sdk.StringVariationDetail(p.FlagKey, context, p.DefaultValue.StringValue())
			result.Value = ldvalue.String(strValue)
		default:
			var jsonValue ldvalue.Value
			jsonValue, result, _ = c.sdk.JSONVariationDetail(p.FlagKey, context, p.DefaultValue)
			result.Value = jsonValue
		}
	} else {
		switch p.ValueType {
		case servicedef.ValueTypeBool:
			var boolValue bool
			boolValue, _ = c.sdk.BoolVariation(p.FlagKey, context, p.DefaultValue.BoolValue())
			result.Value = ldvalue.Bool(boolValue)
		case servicedef.ValueTypeInt:
			var intValue int
			intValue, _ = c.sdk.IntVariation(p.FlagKey, context, p.DefaultValue.IntValue())
			result.Value = ldvalue.Int(intValue)
		case servicedef.ValueTypeDouble:
			var floatValue float64
			floatValue, _ = c.sdk.Float64Variation(p.FlagKey, context, p.DefaultValue.Float64Value())
			result.Value = ldvalue.Float64(floatValue)
		case servicedef.ValueTypeString:
			var strValue string
			strValue, _ = c.sdk.StringVariation(p.FlagKey, context, p.DefaultValue.StringValue())
			result.Value = ldvalue.String(strValue)
		default:
			result.Value, _ = c.sdk.JSONVariation(p.FlagKey, context, p.DefaultValue)
		}
	}
	rep := &servicedef.EvaluateFlagResponse{
		Value:          result.Value,
		VariationIndex: result.VariationIndex.AsPointer(),
	}
	if result.Reason.IsDefined() {
		rep.Reason = &result.Reason
	}
	return rep, nil
}

func (c *SDKClientEntity) evaluateAllFlags(p servicedef.EvaluateAllFlagsParams) (*servicedef.EvaluateAllFlagsResponse, error) {
	var options []flagstate.Option
	if p.ClientSideOnly {
		options = append(options, flagstate.OptionClientSideOnly())
	}
	if p.DetailsOnlyForTrackedFlags {
		options = append(options, flagstate.OptionDetailsOnlyForTrackedFlags())
	}
	if p.WithReasons {
		options = append(options, flagstate.OptionWithReasons())
	}

	flagsState := c.sdk.AllFlagsState(p.Context, options...)
	flagsJSON, _ := json.Marshal(flagsState)
	var mapOut map[string]ldvalue.Value
	_ = json.Unmarshal(flagsJSON, &mapOut)
	return &servicedef.EvaluateAllFlagsResponse{State: mapOut}, nil
}

func (c *SDKClientEntity) contextBuild(p servicedef.ContextBuildParams) (*servicedef.ContextBuildResponse, error) {
	// This method never returns an error, because all inputs are considered valid for this command; failure to
	// build or serialize the Context is an expected condition in some test cases, so the error is just part of
	// the output.
	var context ldcontext.Context
	if p.Single == nil {
		builder := ldcontext.NewMultiBuilder()
		for _, c := range p.Multi {
			builder.Add(makeSingleContext(c))
		}
		context = builder.Build()
	} else {
		context = makeSingleContext(*p.Single)
	}
	if context.Err() != nil {
		return &servicedef.ContextBuildResponse{Error: "build context failed: " + context.Err().Error()}, nil
	}
	data, err := json.Marshal(context)
	if err != nil {
		return &servicedef.ContextBuildResponse{Error: "marshaling failed: " + err.Error()}, nil
	}
	return &servicedef.ContextBuildResponse{Output: string(data)}, nil
}

func makeSingleContext(p servicedef.ContextBuildSingleParams) ldcontext.Context {
	b := ldcontext.NewBuilder(p.Key)
	if p.Kind != nil {
		b.Kind(ldcontext.Kind(*p.Kind))
	}
	if p.Name != nil {
		b.Name(*p.Name)
	}
	if p.Anonymous != nil {
		b.Anonymous(*p.Anonymous)
	}
	for _, attr := range p.Private {
		b.Private(attr)
	}
	for k, v := range p.Custom {
		b.SetValue(k, v)
	}
	return b.Build()
}

func (c *SDKClientEntity) contextConvert(p servicedef.ContextConvertParams) (*servicedef.ContextBuildResponse, error) {
	// This method never returns an error, because all inputs are considered valid for this command; failure to
	// parse or serialize the Context is an expected condition in some test cases, so the error is just part of
	// the output.
	var context ldcontext.Context
	err := json.Unmarshal([]byte(p.Input), &context)
	if err != nil {
		return &servicedef.ContextBuildResponse{Error: "unmarshaling failed: " + err.Error()}, nil
	}
	data, err := json.Marshal(context)
	if err != nil {
		return &servicedef.ContextBuildResponse{Error: "re-marshaling failed: " + err.Error()}, nil
	}
	return &servicedef.ContextBuildResponse{Output: string(data)}, nil
}

func (c *SDKClientEntity) migrationOperation(p servicedef.MigrationOperationParams) (*servicedef.MigrationOperationResponse, error) {
	builder := ld.Migration(c.sdk)
	builder.ReadExecutionOrder(p.ReadExecutionOrder)

	builder.TrackLatency(p.TrackLatency)
	builder.TrackErrors(p.TrackErrors)

	readEndpoint := func(endpoint string) func(interface{}) (interface{}, error) {
		return func(payload interface{}) (interface{}, error) {
			var reader io.Reader
			if val, ok := payload.(*string); ok && val != nil {
				reader = strings.NewReader(*val)
			}
			response, err := http.Post(endpoint, "application/json", reader)

			if err != nil {
				return nil, err
			}

			if response.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("request failed with status code %d", response.StatusCode)
			}

			body, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}

			return string(body), nil
		}
	}

	writeEndpoint := func(endpoint string) func(interface{}) (interface{}, error) {
		return func(payload interface{}) (interface{}, error) {
			var reader io.Reader
			if val, ok := payload.(*string); ok && val != nil {
				reader = strings.NewReader(*val)
			}
			response, err := http.Post(endpoint, "application/json", reader)

			if err != nil {
				return nil, err
			}

			if response.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("request failed with status code %d", response.StatusCode)
			}

			body, err := io.ReadAll(response.Body)
			if err != nil {
				return nil, err
			}

			return string(body), nil
		}
	}

	if p.TrackConsistency {
		var comparsionFunction ld.MigrationComparisonFn = func(lhs, rhs interface{}) bool {
			l := lhs.(string)
			r := rhs.(string)
			return l == r
		}
		builder.Read(readEndpoint(p.OldEndpoint), readEndpoint(p.NewEndpoint), &comparsionFunction)
	} else {
		builder.Read(readEndpoint(p.OldEndpoint), readEndpoint(p.NewEndpoint), nil)
	}

	builder.Write(writeEndpoint(p.OldEndpoint), writeEndpoint(p.NewEndpoint))

	migrator, err := builder.Build()

	if err != nil {
		return nil, err
	}

	if p.Operation == ldmigration.Read {
		result := migrator.Read(p.Key, p.Context, p.DefaultStage, p.Payload)
		if err := result.GetError(); err != nil {
			return &servicedef.MigrationOperationResponse{Result: err.Error()}, nil
		}

		return &servicedef.MigrationOperationResponse{Result: result.GetResult()}, nil
	}

	result := migrator.Write(p.Key, p.Context, p.DefaultStage, p.Payload)
	if err := result.GetAuthoritativeResult().GetError(); err != nil {
		return &servicedef.MigrationOperationResponse{Result: err.Error()}, nil
	}

	return &servicedef.MigrationOperationResponse{Result: result.GetAuthoritativeResult().GetResult()}, nil
}

func makeSDKConfig(config servicedef.SDKConfigParams, sdkLog ldlog.Loggers) ld.Config {
	ret := ld.Config{}
	ret.Logging = ldcomponents.Logging().Loggers(sdkLog)

	if config.ServiceEndpoints != nil {
		ret.ServiceEndpoints.Streaming = config.ServiceEndpoints.Streaming
		ret.ServiceEndpoints.Polling = config.ServiceEndpoints.Polling
		ret.ServiceEndpoints.Events = config.ServiceEndpoints.Events
	}

	if config.Streaming != nil {
		if config.Streaming.BaseURI != "" {
			ret.ServiceEndpoints.Streaming = config.Streaming.BaseURI
		}
		builder := ldcomponents.StreamingDataSource()
		if config.Streaming.InitialRetryDelayMS != nil {
			builder.InitialReconnectDelay(time.Millisecond * time.Duration(*config.Streaming.InitialRetryDelayMS))
		}
		if config.Streaming.Filter.IsDefined() {
			builder.PayloadFilter(config.Streaming.Filter.String())
		}
		ret.DataSource = builder
	} else if config.Polling != nil {
		if config.Polling.BaseURI != "" {
			ret.ServiceEndpoints.Polling = config.Polling.BaseURI
		}
		builder := ldcomponents.PollingDataSource()
		if config.Polling.PollIntervalMS != nil {
			builder.PollInterval(time.Millisecond * time.Duration(*config.Polling.PollIntervalMS))
		}
		if config.Polling.Filter.IsDefined() {
			builder.PayloadFilter(config.Polling.Filter.String())
		}
		ret.DataSource = builder
	}

	if config.Events != nil {
		if config.Events.BaseURI != "" {
			ret.ServiceEndpoints.Events = config.Events.BaseURI
		}
		builder := ldcomponents.SendEvents().
			AllAttributesPrivate(config.Events.AllAttributesPrivate)
		for _, a := range config.Events.GlobalPrivateAttributes {
			builder.PrivateAttributes(a)
		}
		if config.Events.Capacity.IsDefined() {
			builder.Capacity(config.Events.Capacity.IntValue())
		}
		if config.Events.FlushIntervalMS.IsDefined() {
			builder.FlushInterval(time.Millisecond * time.Duration(config.Events.FlushIntervalMS))
		}
		if config.Events.OmitAnonymousContexts {
			builder.OmitAnonymousContexts(true)
		}
		ret.Events = builder
		ret.DiagnosticOptOut = !config.Events.EnableDiagnostics
	} else {
		ret.Events = ldcomponents.NoEvents()
	}

	if config.BigSegments != nil {
		fixture := &BigSegmentStoreFixture{service: &callbackService{baseURL: config.BigSegments.CallbackURI}}
		builder := ldcomponents.BigSegments(fixture)
		if config.BigSegments.UserCacheSize.IsDefined() {
			builder.ContextCacheSize(config.BigSegments.UserCacheSize.IntValue())
		}
		if config.BigSegments.UserCacheTimeMS.IsDefined() {
			builder.ContextCacheTime(time.Millisecond * time.Duration(config.BigSegments.UserCacheTimeMS))
		}
		if config.BigSegments.StaleAfterMS.IsDefined() {
			builder.StaleAfter(time.Millisecond * time.Duration(config.BigSegments.StaleAfterMS))
		}
		if config.BigSegments.StatusPollIntervalMS.IsDefined() {
			builder.StatusPollInterval(time.Millisecond * time.Duration(config.BigSegments.StatusPollIntervalMS))
		}
		ret.BigSegments = builder
	}

	if config.Tags != nil {
		ret.ApplicationInfo = interfaces.ApplicationInfo{
			ApplicationID:      config.Tags.ApplicationID.StringValue(),
			ApplicationVersion: config.Tags.ApplicationVersion.StringValue(),
		}
	}

	if config.Hooks != nil {
		hooks := make([]ldhooks.Hook, 0)
		for _, hookConfig := range config.Hooks.Hooks {
			hookInstance := newTestHook(hookConfig.Name, hookConfig.CallbackURI, hookConfig.Data, hookConfig.Errors)
			hooks = append(hooks, hookInstance)
		}
		ret.Hooks = hooks
	}

	return ret
}

func asJSON(value interface{}) string {
	ret, _ := json.Marshal(value)
	return string(ret)
}
