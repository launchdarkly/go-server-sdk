package ldai

import (
	"encoding/json"
	"fmt"
	"github.com/alexkappa/mustache"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/internal/datamodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

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

type Client struct {
	sdk    ServerSDK
	logger interfaces.LDLoggers
}

const ldContextVariable = "ldctx"

func New(sdk ServerSDK) (*Client, error) {
	if sdk == nil {
		return nil, fmt.Errorf("sdk must not be nil")
	}
	return &Client{
		sdk:    sdk,
		logger: sdk.Loggers(),
	}, nil
}

func (c *Client) logConfigWarning(key string, format string, args ...interface{}) {
	prefix := "AI config '" + key + "': "
	c.logger.Warnf(prefix+format, args...)
}

func (c *Client) Config(
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
		return defaultValue, NewTracker(key, c.sdk, &defaultValue, context, c.logger)
	}

	var parsed datamodel.Config
	if err := json.Unmarshal([]byte(result.JSONString()), &parsed); err != nil {
		c.logConfigWarning(key, "unmarshalling failed: %v", err)
		return defaultValue, NewTracker(key, c.sdk, &defaultValue, context, c.logger)
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
		WithModelId(parsed.Model.Id).
		WithProviderId(parsed.Provider.Id).
		WithEnabled(parsed.Meta.Enabled)

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
	return cfg, NewTracker(key, c.sdk, &cfg, context, c.logger)
}

func getAllAttributes(context ldcontext.Context) map[string]interface{} {
	if !context.Multiple() {
		return addSingleKindContextAttributes(context)
	}

	attributes := map[string]interface{}{
		"kind": context.Kind(),
		"key":  context.FullyQualifiedKey(),
	}

	for _, ctx := range context.GetAllIndividualContexts(nil) {
		attributes[string(ctx.Kind())] = addSingleKindContextAttributes(ctx)
	}

	return attributes
}

func addSingleKindContextAttributes(context ldcontext.Context) map[string]interface{} {
	attributes := map[string]interface{}{
		"kind":      context.Kind(),
		"key":       context.Key(),
		"anonymous": context.Anonymous(),
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
