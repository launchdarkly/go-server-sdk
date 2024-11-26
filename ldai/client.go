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

func (c *Client) Config(
	key string,
	context ldcontext.Context,
	defaultValue Config,
	variables map[string]interface{},
) (Config, *Tracker) {
	variation, err := c.sdk.JSONVariation(key, context, ldvalue.Null())
	if err != nil {
		c.logger.Warnf("Error fetching JSON variation: %s", err.Error())
		return defaultValue, NewTracker(&defaultValue)
	}

	if variation.IsNull() {
		c.logger.Warnf("JSON variation was null")
		return defaultValue, NewTracker(&defaultValue)
	}

	var parsed datamodel.Config
	if err := json.Unmarshal([]byte(variation.JSONString()), &parsed); err != nil {
		c.logger.Warnf("Error unmarshalling JSON variation: %s", err.Error())
		return defaultValue, NewTracker(&defaultValue)
	}

	mergedVariables := map[string]interface{}{
		ldContextVariable: getAllAttributes(context),
	}
	for k, v := range variables {
		if k == ldContextVariable {
			c.logger.Warnf("AI model config variables contains 'ldctx' key, which is reserved")
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
			c.logger.Errorf(
				"Malformed message at index %d: %s", i, err.Error(),
			)
			return defaultValue, &Tracker{}
		}
		builder.WithMessage(content, msg.Role)
	}

	cfg := builder.Build()
	return cfg, NewTracker(&cfg)
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
