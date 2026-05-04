package ldai

import (
	"encoding/json"
	"fmt"

	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"

	"github.com/alexkappa/mustache"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

// Defines the Mustache variable name used to access the provided context.
const ldContextVariable = "ldctx"

// JudgePlaceholderMessageHistory and JudgePlaceholderResponseToEvaluate are the literal placeholder
// strings shared between JudgeConfig (pass 1) and Judge.buildMessages (pass 2). Both must use the
// same values or substitution silently fails.
const (
	JudgePlaceholderMessageHistory     = "{{message_history}}"
	JudgePlaceholderResponseToEvaluate = "{{response_to_evaluate}}"
)

// ServerSDK defines the required methods for the AI SDK to interact with LaunchDarkly. These methods are
// satisfied by the LaunchDarkly Go Server SDK.
type ServerSDK interface {
	JSONVariation(
		key string,
		context ldcontext.Context,
		defaultVal ldvalue.Value,
	) (ldvalue.Value, error)
	Loggers() interfaces.LDLoggers
	TrackMetric(
		eventName string,
		context ldcontext.Context,
		metricValue float64,
		data ldvalue.Value,
	) error
}

// Client is the main entrypoint for the AI SDK. A client can be used to obtain an AI Config from LaunchDarkly.
// Unless otherwise noted, the Client's method are not safe for concurrent use.
type Client struct {
	sdk    ServerSDK
	logger interfaces.LDLoggers
}

const (
	sdkInfoEvent          = "$ld:ai:sdk:info"
	usageCompletionConfig = "$ld:ai:usage:completion-config"
	usageJudgeConfig      = "$ld:ai:usage:judge-config"
)

// NewClient creates a new AI Client. The provided SDK interface must not be nil. The client will use the provided SDK's
// loggers to log warnings and errors.
func NewClient(sdk ServerSDK) (*Client, error) {
	if sdk == nil {
		return nil, fmt.Errorf("sdk must not be nil")
	}
	c := &Client{
		sdk:    sdk,
		logger: sdk.Loggers(),
	}
	if err := c.trackSDKInfo(); err != nil {
		c.logger.Warnf("AI Client: failed to track SDK info: %v", err)
	}
	return c, nil
}

func (c *Client) trackSDKInfo() error {
	ctx, err := ldcontext.NewBuilder("ld-internal-tracking").Kind("ld_ai").Anonymous(true).TryBuild()
	if err != nil {
		return err
	}
	data := ldvalue.ObjectBuild().
		Set("aiSdkName", ldvalue.String(SDKName)).
		Set("aiSdkVersion", ldvalue.String(Version)).
		Set("aiSdkLanguage", ldvalue.String(SDKLanguage)).
		Build()
	return c.sdk.TrackMetric(sdkInfoEvent, ctx, 1, data)
}

func (c *Client) logConfigWarning(key string, format string, args ...interface{}) {
	prefix := "AI Config '" + key + "': "
	c.logger.Warnf(prefix+format, args...)
}

// CompletionConfig retrieves an AI Config and interpolates its message templates using the provided
// variables. Returns the default value if the config cannot be evaluated. Template interpolation is
// not applied to the default value's messages.
//
// To send analytic events to LaunchDarkly, call methods on the returned Tracker.
func (c *Client) CompletionConfig(
	key string,
	context ldcontext.Context,
	defaultValue Config,
	variables map[string]interface{},
) (Config, *Tracker) {
	data := ldvalue.ObjectBuild().Set("configKey", ldvalue.String(key)).Build()
	_ = c.sdk.TrackMetric(usageCompletionConfig, context, 1, data)
	return c.evaluateConfig(key, context, defaultValue, variables)
}

// Config evaluates an AI Config named by a given key for the given context.
//
// Deprecated: Use CompletionConfig instead.
func (c *Client) Config(
	key string,
	context ldcontext.Context,
	defaultValue Config,
	variables map[string]interface{},
) (Config, *Tracker) {
	return c.CompletionConfig(key, context, defaultValue, variables)
}

// evaluateConfig fetches and interpolates an AI Config without emitting any metric.
// Callers (Config, JudgeConfig) are meant to emit their own metric before calling this.
func (c *Client) evaluateConfig(
	key string,
	context ldcontext.Context,
	defaultValue Config,
	variables map[string]interface{},
) (Config, *Tracker) {
	result, _ := c.sdk.JSONVariation(key, context, defaultValue.AsLdValue())

	// The spec requires the config to at least be an object (although all properties are optional, so it may be an
	// empty object.)
	if result.Type() != ldvalue.ObjectType {
		c.logConfigWarning(key, "unmarshalling failed, expected JSON object but got %s", result.Type().String())
		return defaultValue, newTracker(key, "", 1, c.sdk, &defaultValue, context, c.logger)
	}

	var parsed datamodel.Config
	if err := json.Unmarshal([]byte(result.JSONString()), &parsed); err != nil {
		c.logConfigWarning(key, "unmarshalling failed: %v", err)
		return defaultValue, newTracker(key, "", 1, c.sdk, &defaultValue, context, c.logger)
	}

	mergedVariables := map[string]interface{}{
		ldContextVariable: getAllAttributes(context),
	}

	for k, v := range variables {
		if k == ldContextVariable {
			c.logConfigWarning(key, "config variables contains 'ldctx', which is reserved and cannot be overwritten")
			continue
		}
		mergedVariables[k] = v
	}

	builder := NewConfig().
		withVariationKey(parsed.Meta.VariationKey).
		WithModelName(parsed.Model.Name).
		WithProviderName(parsed.Provider.Name).
		WithEnabled(parsed.Meta.Enabled).
		WithMode(parsed.Mode).
		WithEvaluationMetricKey(parsed.EvaluationMetricKey).
		WithEvaluationMetricKeys(parsed.EvaluationMetricKeys).
		WithJudgeConfiguration(parsed.JudgeConfiguration)

	if parsed.Meta.Version != nil {
		builder.withVersion(*parsed.Meta.Version)
	}

	for k, v := range parsed.Model.Parameters {
		builder.WithModelParam(k, v)
	}

	for k, v := range parsed.Model.Custom {
		builder.WithCustomModelParam(k, v)
	}

	for i, msg := range parsed.Messages {
		content, err := interpolateTemplate(msg.Content, mergedVariables)
		if err != nil {
			c.logConfigWarning(key,
				"malformed message at index %d: %v", i, err,
			)
			return defaultValue, &Tracker{}
		}
		builder.WithMessage(content, msg.Role)
	}

	cfg := builder.Build()

	return cfg, newTracker(key, cfg.VariationKey(), cfg.Version(), c.sdk, &cfg, context, c.logger)
}

func getAllAttributes(context ldcontext.Context) map[string]interface{} {
	if !context.Multiple() {
		return addContextAttributes(context, false)
	}

	attributes := map[string]interface{}{
		"kind": context.Kind(),
		"key":  context.FullyQualifiedKey(),
	}

	for _, ctx := range context.GetAllIndividualContexts(nil) {
		attributes[string(ctx.Kind())] = addContextAttributes(ctx, true)
	}

	return attributes
}

func addContextAttributes(context ldcontext.Context, omitKind bool) map[string]interface{} {
	attributes := map[string]interface{}{
		"key":       context.Key(),
		"anonymous": context.Anonymous(),
	}

	if !omitKind {
		attributes["kind"] = context.Kind()
	}

	for _, attr := range context.GetOptionalAttributeNames(nil) {
		attributes[attr] = context.GetValue(attr).AsArbitraryValue()
	}

	return attributes
}

func interpolateTemplate(template string, variables map[string]interface{}) (string, error) {
	m := mustache.New()
	if err := m.ParseString(template); err != nil {
		return "", err
	}
	return m.RenderString(variables)
}

// JudgeConfig retrieves a Judge AI Config and interpolates its message templates. The reserved
// variables message_history and response_to_evaluate are preserved as literal placeholders for
// substitution by Judge.buildMessages during evaluation.
//
// To send analytic events to LaunchDarkly, call methods on the returned Tracker.
func (c *Client) JudgeConfig(
	key string,
	context ldcontext.Context,
	defaultValue Config,
	variables map[string]interface{},
) (Config, *Tracker) {
	data := ldvalue.ObjectBuild().Set("configKey", ldvalue.String(key)).Build()
	_ = c.sdk.TrackMetric(usageJudgeConfig, context, 1, data)

	// Extend variables with reserved judge placeholders
	extendedVariables := make(map[string]interface{})
	for k, v := range variables {
		// Warn if user tries to override reserved variables
		if k == "message_history" || k == "response_to_evaluate" {
			c.logger.Warnf("AI Config '%s': variable '%s' is reserved by judge and will be ignored", key, k)
			continue
		}
		extendedVariables[k] = v
	}

	// Inject reserved variables as literal placeholder strings
	// These will be preserved through the first interpolation and resolved during Judge.Evaluate()
	extendedVariables["message_history"] = JudgePlaceholderMessageHistory
	extendedVariables["response_to_evaluate"] = JudgePlaceholderResponseToEvaluate

	return c.evaluateConfig(key, context, defaultValue, extendedVariables)
}
