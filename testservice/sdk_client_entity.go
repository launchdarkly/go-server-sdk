package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	ld "gopkg.in/launchdarkly/go-server-sdk.v5"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testservice/servicedef"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
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
	c.sdk.Close()
	c.logger.Println("Test ended")
	c.logger.SetOutput(ioutil.Discard)
}

func (c *SDKClientEntity) DoCommand(params servicedef.CommandParams) (interface{}, error) {
	c.logger.Printf("Test service sent command: %s", asJSON(params))
	switch params.Command {
	case servicedef.CommandEvaluateFlag:
		return c.evaluateFlag(*params.Evaluate)
	case servicedef.CommandEvaluateAllFlags:
		return c.evaluateAllFlags(*params.EvaluateAll)
	case servicedef.CommandIdentifyEvent:
		err := c.sdk.Identify(params.IdentifyEvent.User)
		return nil, err
	case servicedef.CommandCustomEvent:
		if params.CustomEvent.MetricValue != nil {
			return nil, c.sdk.TrackMetric(params.CustomEvent.EventKey, *params.CustomEvent.User,
				*params.CustomEvent.MetricValue, params.CustomEvent.Data)
		}
		if params.CustomEvent.Data.IsDefined() {
			return nil, c.sdk.TrackData(params.CustomEvent.EventKey, *params.CustomEvent.User, params.CustomEvent.Data)
		}
		return nil, c.sdk.TrackEvent(params.CustomEvent.EventKey, *params.CustomEvent.User)
	case servicedef.CommandAliasEvent:
		err := c.sdk.Alias(params.AliasEvent.User, params.AliasEvent.PreviousUser)
		return nil, err
	case servicedef.CommandFlushEvents:
		c.sdk.Flush()
		return nil, nil
	case servicedef.CommandGetBigSegmentStoreStatus:
		bigSegmentsStatus := c.sdk.GetBigSegmentStoreStatusProvider().GetStatus()
		return servicedef.BigSegmentStoreStatusResponse(bigSegmentsStatus), nil
	default:
		return nil, BadRequestError{Message: fmt.Sprintf("unknown command %q", params.Command)}
	}
}

func (c *SDKClientEntity) evaluateFlag(p servicedef.EvaluateFlagParams) (*servicedef.EvaluateFlagResponse, error) {
	if p.User == nil {
		return nil, BadRequestError{"user is required for server-side evaluations"}
	}

	var result ldreason.EvaluationDetail
	if p.Detail {
		switch p.ValueType {
		case servicedef.ValueTypeBool:
			var boolValue bool
			boolValue, result, _ = c.sdk.BoolVariationDetail(p.FlagKey, *p.User, p.DefaultValue.BoolValue())
			result.Value = ldvalue.Bool(boolValue)
		case servicedef.ValueTypeInt:
			var intValue int
			intValue, result, _ = c.sdk.IntVariationDetail(p.FlagKey, *p.User, p.DefaultValue.IntValue())
			result.Value = ldvalue.Int(intValue)
		case servicedef.ValueTypeDouble:
			var floatValue float64
			floatValue, result, _ = c.sdk.Float64VariationDetail(p.FlagKey, *p.User, p.DefaultValue.Float64Value())
			result.Value = ldvalue.Float64(floatValue)
		case servicedef.ValueTypeString:
			var strValue string
			strValue, result, _ = c.sdk.StringVariationDetail(p.FlagKey, *p.User, p.DefaultValue.StringValue())
			result.Value = ldvalue.String(strValue)
		default:
			var jsonValue ldvalue.Value
			jsonValue, result, _ = c.sdk.JSONVariationDetail(p.FlagKey, *p.User, p.DefaultValue)
			result.Value = jsonValue
		}
	} else {
		switch p.ValueType {
		case servicedef.ValueTypeBool:
			var boolValue bool
			boolValue, _ = c.sdk.BoolVariation(p.FlagKey, *p.User, p.DefaultValue.BoolValue())
			result.Value = ldvalue.Bool(boolValue)
		case servicedef.ValueTypeInt:
			var intValue int
			intValue, _ = c.sdk.IntVariation(p.FlagKey, *p.User, p.DefaultValue.IntValue())
			result.Value = ldvalue.Int(intValue)
		case servicedef.ValueTypeDouble:
			var floatValue float64
			floatValue, _ = c.sdk.Float64Variation(p.FlagKey, *p.User, p.DefaultValue.Float64Value())
			result.Value = ldvalue.Float64(floatValue)
		case servicedef.ValueTypeString:
			var strValue string
			strValue, _ = c.sdk.StringVariation(p.FlagKey, *p.User, p.DefaultValue.StringValue())
			result.Value = ldvalue.String(strValue)
		default:
			result.Value, _ = c.sdk.JSONVariation(p.FlagKey, *p.User, p.DefaultValue)
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
	if p.User == nil {
		return nil, BadRequestError{"user is required for server-side evaluations"}
	}

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

	flagsState := c.sdk.AllFlagsState(*p.User, options...)
	flagsJSON, _ := json.Marshal(flagsState)
	var mapOut map[string]ldvalue.Value
	_ = json.Unmarshal(flagsJSON, &mapOut)
	return &servicedef.EvaluateAllFlagsResponse{State: mapOut}, nil
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
		ret.DataSource = builder
	} else if config.Polling != nil {
		if config.Polling.BaseURI != "" {
			ret.ServiceEndpoints.Polling = config.Polling.BaseURI
		}
		builder := ldcomponents.PollingDataSource()
		if config.Polling.PollIntervalMS != nil {
			builder.PollInterval(time.Millisecond * time.Duration(*config.Polling.PollIntervalMS))
		}
		ret.DataSource = builder
	}

	if config.Events != nil {
		if config.Events.BaseURI != "" {
			ret.ServiceEndpoints.Events = config.Events.BaseURI
		}
		builder := ldcomponents.SendEvents().
			AllAttributesPrivate(config.Events.AllAttributesPrivate).
			PrivateAttributeNames(config.Events.GlobalPrivateAttributes...).
			InlineUsersInEvents(config.Events.InlineUsers)
		if config.Events.Capacity.IsDefined() {
			builder.Capacity(config.Events.Capacity.IntValue())
		}
		if config.Events.FlushIntervalMS.IsDefined() {
			builder.FlushInterval(time.Millisecond * time.Duration(config.Events.FlushIntervalMS))
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
			builder.UserCacheSize(config.BigSegments.UserCacheSize.IntValue())
		}
		if config.BigSegments.UserCacheTimeMS.IsDefined() {
			builder.UserCacheTime(time.Millisecond * time.Duration(config.BigSegments.UserCacheTimeMS))
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

	return ret
}

func asJSON(value interface{}) string {
	ret, _ := json.Marshal(value)
	return string(ret)
}
