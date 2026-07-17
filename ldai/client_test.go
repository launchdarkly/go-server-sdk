package ldai

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/launchdarkly/go-server-sdk/ldai/datamodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
)

type mockServerSDK struct {
	log    *ldlogtest.MockLog
	json   []byte
	err    error
	events []mockEvent
}

type mockEvent struct {
	eventName   string
	context     ldcontext.Context
	metricValue float64
	data        ldvalue.Value
}

func newMockSDK(json []byte, err error) *mockServerSDK {
	return &mockServerSDK{json: json, err: err, log: ldlogtest.NewMockLog(), events: []mockEvent{}}
}

func (m *mockServerSDK) JSONVariation(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (ldvalue.Value, error) {

	if m.err != nil {
		return defaultVal, m.err
	}

	return ldvalue.Parse(m.json), nil
}

func (m *mockServerSDK) Loggers() interfaces.LDLoggers {
	return m.log.Loggers
}

func (m *mockServerSDK) TrackMetric(eventName string, context ldcontext.Context, metricValue float64, data ldvalue.Value) error {
	m.events = append(m.events, mockEvent{
		eventName:   eventName,
		context:     context,
		metricValue: metricValue,
		data:        data,
	})
	return nil
}

func TestNewClientReturnsErrorWhenSDKIsNil(t *testing.T) {
	_, err := NewClient(nil)
	require.Error(t, err)
}

func TestNewClient(t *testing.T) {
	mockSDK := newMockSDK(nil, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Verify SDK info event was fired on construction.
	require.Len(t, mockSDK.events, 1)
	evt := mockSDK.events[0]
	assert.Equal(t, "$ld:ai:sdk:info", evt.eventName)
	assert.Equal(t, float64(1), evt.metricValue)
	assert.Equal(t, "go-server-sdk/ldai", evt.data.GetByKey("aiSdkName").StringValue())
	assert.Equal(t, Version, evt.data.GetByKey("aiSdkVersion").StringValue())
	assert.Equal(t, "go", evt.data.GetByKey("aiSdkLanguage").StringValue())

	// Verify the context is anonymous with kind ld_ai.
	assert.Equal(t, "ld-internal-tracking", evt.context.Key())
	assert.True(t, evt.context.Anonymous())
	assert.Equal(t, ldcontext.Kind("ld_ai"), evt.context.Kind())
}

func TestEvalErrorReturnsDefault(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, errors.New("client is offline")))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()

	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)
	assert.NotNil(t, cfg.CreateTracker())
	assert.Equal(t, defaultVal.Enabled(), cfg.Enabled())
	assert.Equal(t, defaultVal.Messages(), cfg.Messages())
	assert.Equal(t, defaultVal.ModelName(), cfg.ModelName())
	assert.Equal(t, defaultVal.ProviderName(), cfg.ProviderName())
}

func TestParseMultipleMessages(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"messages": [
			{"content": "hello", "role": "user"},
			{"content": "world", "role": "system"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	assert.ElementsMatch(t, cfg.Messages(), []datamodel.Message{
		{Content: "hello", Role: datamodel.User},
		{Content: "world", Role: datamodel.System},
	})
}

func TestParseModelName(t *testing.T) {
	tests := []struct {
		name     string
		json     []byte
		expected string
	}{
		{"missing", []byte(`{"model": {}}`), ""},
		{"empty string", []byte(`{"model": {"name": ""}}`), ""},
		{"non-empty string", []byte(`{"model": {"name": "my-model"}}`), "my-model"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(newMockSDK(test.json, nil))
			require.NoError(t, err)
			require.NotNil(t, client)

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

			assert.Equal(t, test.expected, cfg.ModelName())
		})
	}
}

func TestParseModelKeyAndVersion(t *testing.T) {
	// modelKey/modelVersion are intentionally not exposed on Config (they'd read as properties of
	// the LLM itself, e.g. a version like "5.4"); the only place they surface is the tracker's
	// stamped event data, mirroring variationKey/version.
	tests := []struct {
		name            string
		json            []byte
		expectedKey     string
		expectedVersion int
	}{
		{
			name:            "missing",
			json:            []byte(`{"model": {"name": "gpt-4"}}`),
			expectedKey:     "",
			expectedVersion: 1,
		},
		{
			name:            "modelKey and modelVersion set",
			json:            []byte(`{"model": {"name": "gpt-4"}, "_ldMeta": {"modelKey": "my-model", "modelVersion": 2}}`),
			expectedKey:     "my-model",
			expectedVersion: 2,
		},
		{
			name:            "modelVersion only",
			json:            []byte(`{"model": {"name": "gpt-4"}, "_ldMeta": {"modelVersion": 3}}`),
			expectedKey:     "",
			expectedVersion: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockSDK := newMockSDK(test.json, nil)
			client, err := NewClient(mockSDK)
			require.NoError(t, err)
			require.NotNil(t, client)
			mockSDK.events = nil

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)
			tracker := cfg.CreateTracker()
			require.NotNil(t, tracker)
			assert.NoError(t, tracker.TrackSuccess())

			require.NotEmpty(t, mockSDK.events)
			data := mockSDK.events[len(mockSDK.events)-1].data
			assert.Equal(t, test.expectedKey, data.GetByKey("modelKey").StringValue())
			assert.Equal(t, test.expectedVersion, data.GetByKey("modelVersion").IntValue())
		})
	}
}

func TestCreateTrackerStampsModelKeyAndVersionOnTrackData(t *testing.T) {
	configJSON := []byte(`{
		"_ldMeta": {"variationKey": "var-1", "enabled": true, "version": 1, "modelKey": "my-model", "modelVersion": 2},
		"model": {"name": "gpt-4"},
		"provider": {"name": "openai"},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	mockSDK := newMockSDK(configJSON, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)
	mockSDK.events = nil

	cfg := client.CompletionConfig("my-config", ldcontext.New("user"), Disabled(), nil)
	tracker := cfg.CreateTracker()
	require.NotNil(t, tracker)
	assert.NoError(t, tracker.TrackSuccess())

	require.NotEmpty(t, mockSDK.events)
	data := mockSDK.events[len(mockSDK.events)-1].data
	assert.Equal(t, "my-model", data.GetByKey("modelKey").StringValue())
	assert.Equal(t, 2, data.GetByKey("modelVersion").IntValue())
}

func TestParseProviderName(t *testing.T) {
	tests := []struct {
		name     string
		json     []byte
		expected string
	}{
		{"missing", []byte(`{"provider": {}}`), ""},
		{"empty string", []byte(`{"provider": {"name": ""}}`), ""},
		{"non-empty string", []byte(`{"provider": {"name": "my-provider"}}`), "my-provider"}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(newMockSDK(test.json, nil))
			require.NoError(t, err)
			require.NotNil(t, client)

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

			assert.Equal(t, test.expected, cfg.ProviderName())
		})
	}
}

func TestParseInvalidConfigReturnsDefault(t *testing.T) {
	tests := []struct {
		name string
		json []byte
	}{
		{"null value", []byte("null")},
		{"invalid json", []byte("invalid")},
		{"is a number", []byte("42")},
		{"is a string", []byte(`"hello"`)},
		{"is an array", []byte(`["hello"]`)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sdk := newMockSDK(test.json, nil)
			client, err := NewClient(sdk)
			require.NoError(t, err)
			require.NotNil(t, client)

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()

			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)
			// Verify config data matches the default
			assert.Equal(t, defaultVal.AsLdValue(), cfg.AsLdValue())
			// Verify CreateTracker() now works (returnDefault always injects a factory)
			assert.NotNil(t, cfg.CreateTracker())

			sdk.log.AssertMessageMatch(t, true, ldlog.Warn, "AI Config 'key':")
		})
	}
}

func TestParseDisabledConfigs(t *testing.T) {
	tests := []struct {
		name string
		json []byte
	}{
		{"empty object", []byte("{}")},
		{"missing meta field", []byte(`{"model": {}, "messages": []}`)},
		{"meta disabled explicitly", []byte(`{"meta": {"enabled": false, "variationKey": "1"}, "model": {}, "messages": []}`)},
		{"meta disable implicitly", []byte(`{"meta": { "variationKey": "1"}, "model": {}, "messages": []}`)},
	}

	defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(newMockSDK(test.json, nil))
			require.NoError(t, err)
			require.NotNil(t, client)

			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

			// We *shouldn't* be getting the default value, because these are all valid configs that should
			// be parsed as disabled.
			assert.False(t, cfg.Enabled())
		})
	}
}

func TestParseModelParams(t *testing.T) {
	tests := []struct {
		name     string
		json     []byte
		expected map[string]ldvalue.Value
	}{
		{"omitted", []byte(`{"model": {"name": "model"}}`), nil},
		{"empty", []byte(`{"model": {"name": "model", "parameters": {}}}`), map[string]ldvalue.Value{}},
		{"single", []byte(`{"model": {"name": "model", "parameters": {"foo": "bar"}}}`),
			map[string]ldvalue.Value{"foo": ldvalue.String("bar")}},
		{"multiple", []byte(`{"model": {"name": "model", "parameters": {"foo": "bar", "baz": 42}}}`),
			map[string]ldvalue.Value{"foo": ldvalue.String("bar"), "baz": ldvalue.Int(42)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(newMockSDK(test.json, nil))
			require.NoError(t, err)
			require.NotNil(t, client)

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

			for k, v := range test.expected {
				p, ok := cfg.ModelParam(k)
				if assert.True(t, ok) {
					assert.Equal(t, v, p)
				}
			}
		})
	}
}

func TestParseCustomModelParams(t *testing.T) {
	tests := []struct {
		name     string
		json     []byte
		expected map[string]ldvalue.Value
	}{
		{"omitted", []byte(`{"model": {"name": "model"}}`), nil},
		{"empty", []byte(`{"model": {"name": "model", "custom": {}}}`), map[string]ldvalue.Value{}},
		{"single", []byte(`{"model": {"name": "model", "custom": {"foo": "bar"}}}`),
			map[string]ldvalue.Value{"foo": ldvalue.String("bar")}},
		{"multiple", []byte(`{"model": {"name": "model", "custom": {"foo": "bar", "baz": 42}}}`),
			map[string]ldvalue.Value{"foo": ldvalue.String("bar"), "baz": ldvalue.Int(42)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(newMockSDK(test.json, nil))
			require.NoError(t, err)
			require.NotNil(t, client)

			defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
			cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

			for k, v := range test.expected {
				p, ok := cfg.CustomModelParam(k)
				if assert.True(t, ok) {
					assert.Equal(t, v, p)
				}
			}
		})
	}
}

func TestCanSetDefaultConfigFields(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().Enable().
		WithMessage("hello", datamodel.User).
		WithMessage("world", datamodel.System).
		WithProviderName("provider").
		WithModelName("model").Build()

	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

	assert.True(t, cfg.Enabled())
	assert.Equal(t, "provider", cfg.ProviderName())
	assert.Equal(t, "model", cfg.ModelName())
	assert.Equal(t, 2, len(cfg.Messages()))

	msg := cfg.Messages()
	assert.Equal(t, "hello", msg[0].Content)
	assert.Equal(t, datamodel.User, msg[0].Role)
	assert.Equal(t, "world", msg[1].Content)
	assert.Equal(t, datamodel.System, msg[1].Role)
}

func TestCompletionConfigMethodTracking(t *testing.T) {
	mockSDK := newMockSDK(nil, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Clear the SDK info event from construction.
	mockSDK.events = nil

	defaultConfig := NewConfig().WithEnabled(false).Build()
	context := ldcontext.New("user-key")
	configKey := "test-config-key"

	config := client.CompletionConfig(configKey, context, defaultConfig, nil)

	require.NotNil(t, config.CreateTracker())

	expectedData := ldvalue.ObjectBuild().Set("configKey", ldvalue.String(configKey)).Build()
	expectedEvents := []mockEvent{
		{
			eventName:   "$ld:ai:usage:completion-config",
			context:     context,
			metricValue: 1,
			data:        expectedData,
		},
	}

	assert.ElementsMatch(t, expectedEvents, mockSDK.events)
}

// TestJudgeConfigMethodTracking verifies that JudgeConfig emits only the judge metric,
// not the completion-config metric, so judge evaluations are not double-counted on the dashboard.
func TestJudgeConfigMethodTracking(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKey": "toxicity",
		"messages": [{"content": "test", "role": "system"}]
	}`)
	mockSDK := newMockSDK(json, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)
	require.NotNil(t, client)

	// Clear the SDK info event from construction.
	mockSDK.events = nil

	defaultConfig := Disabled()
	context := ldcontext.New("user-key")
	configKey := "judge-config-key"

	config := client.JudgeConfig(configKey, context, defaultConfig, nil)

	require.NotNil(t, config.CreateTracker())

	// Only the judge metric should be emitted; evaluateConfig does not emit any metric.
	expectedData := ldvalue.ObjectBuild().Set("configKey", ldvalue.String(configKey)).Build()
	expectedEvents := []mockEvent{
		{
			eventName:   "$ld:ai:usage:judge-config",
			context:     context,
			metricValue: 1,
			data:        expectedData,
		},
	}
	assert.ElementsMatch(t, expectedEvents, mockSDK.events,
		"JudgeConfig must not emit $ld:ai:usage:completion-config to avoid double-counting")
}

func TestCanSetModelParameters(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().WithModelParam("foo", ldvalue.String("bar")).Build()
	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

	t.Run("param is present", func(t *testing.T) {
		p, ok := cfg.ModelParam("foo")
		assert.True(t, ok)
		assert.Equal(t, "bar", p.StringValue())
	})

	t.Run("param is missing", func(t *testing.T) {
		p, ok := cfg.ModelParam("missing")
		assert.False(t, ok)
		assert.Equal(t, ldvalue.Null(), p)
	})
}

func TestCanSetCustomModelParameters(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().WithCustomModelParam("foo", ldvalue.String("bar")).Build()
	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

	t.Run("param is present", func(t *testing.T) {
		p, ok := cfg.CustomModelParam("foo")
		assert.True(t, ok)
		assert.Equal(t, "bar", p.StringValue())
	})

	t.Run("param is missing", func(t *testing.T) {
		p, ok := cfg.CustomModelParam("missing")
		assert.False(t, ok)
		assert.Equal(t, ldvalue.Null(), p)
	})
}

func TestNormalAndCustomParamsDoNotInterfere(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().
		WithModelParam("foo", ldvalue.String("bar")).
		WithCustomModelParam("foo", ldvalue.String("baz")).Build()

	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

	foo1, ok := cfg.ModelParam("foo")
	require.True(t, ok)
	assert.Equal(t, "bar", foo1.StringValue())

	foo2, ok := cfg.CustomModelParam("foo")
	require.True(t, ok)
	assert.Equal(t, "baz", foo2.StringValue())
}

func TestCannotOverwriteMessages(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().
		WithMessage("hello", datamodel.Assistant).Build()

	cfg := client.CompletionConfig("key", ldcontext.New("user"), defaultVal, nil)

	cfg.Messages()[0].Content = "changed"
	cfg.Messages()[0].Role = datamodel.User

	assert.ElementsMatch(t, []datamodel.Message{{Content: "hello", Role: datamodel.Assistant}}, cfg.Messages())
}

func eval(t *testing.T, prompt string, ctx ldcontext.Context, variables map[string]interface{}) (string, error) {
	t.Helper()
	json := []byte(`{
					"_ldMeta": {"variationKey": "1", "enabled": true},
					"messages": [
						{"content": "` + prompt + `", "role": "user"}
					]
				}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	cfg := client.CompletionConfig("key", ctx, Disabled(), variables)
	if len(cfg.Messages()) == 0 {
		return "", errors.New("no messages interpolated")
	}
	return cfg.Messages()[0].Content, nil
}

func TestInterpolation(t *testing.T) {
	t.Run("missing variables", func(t *testing.T) {
		cases := []string{
			"{{ adjective }}",
			"{{ adjective.nested.deep }}",
			"{{ ldctx.this_is_not_a_variable }}",
		}

		for _, c := range cases {
			t.Run(c, func(t *testing.T) {
				result, err := eval(t, "I am an ("+c+") LLM", ldcontext.New("user"), nil)
				require.NoError(t, err)
				assert.Equal(t, "I am an () LLM", result)
			})
		}
	})

	t.Run("simple variables", func(t *testing.T) {
		cases := []string{
			"awesome",
			"slow",
			"all powerful",
		}

		for _, c := range cases {
			t.Run(c, func(t *testing.T) {
				result, err := eval(t, "I am an {{ adjective }} LLM", ldcontext.New("user"), map[string]interface{}{"adjective": c})
				require.NoError(t, err)
				assert.Equal(t, "I am an "+c+" LLM", result)
			})
		}
	})

	t.Run("multiple variables", func(t *testing.T) {
		vars := map[string]interface{}{
			"adjective": "awesome",
			"noun":      "robot",
			"stats": map[string]interface{}{
				"power": "9000",
			},
		}
		result, err := eval(t, "I am an {{ adjective }} {{ noun }} with power over {{ stats.power }}", ldcontext.New("user"), vars)
		require.NoError(t, err)
		assert.Equal(t, "I am an awesome robot with power over 9000", result)
	})

	t.Run("interpolation with array indices does not work", func(t *testing.T) {
		vars := map[string]interface{}{
			"adjectives": []string{"awesome", "slow", "all powerful"},
		}

		t.Run("dot syntax interpolates as empty string", func(t *testing.T) {
			result, err := eval(t, "I am an ({{ adjectives.0 }}) LLM", ldcontext.New("user"), vars)
			require.NoError(t, err)
			assert.Equal(t, "I am an () LLM", result)
		})

		t.Run("bracket syntax returns error", func(t *testing.T) {
			_, err := eval(t, "I am an ({{ adjectives[0] }}) LLM", ldcontext.New("user"), vars)
			assert.Error(t, err)
		})
	})

	t.Run("array sections", func(t *testing.T) {
		vars := map[string]interface{}{
			"adjectives": []string{"hello", "world", "!"},
		}

		result, err := eval(t, "{{#adjectives }}{{ . }} {{/adjectives }}", ldcontext.New("user"), vars)
		require.NoError(t, err)
		assert.Equal(t, "hello world ! ", result)
	})

	t.Run("malformed syntax", func(t *testing.T) {
		_, err := eval(t, "This is a {{ malformed }]} prompt", ldcontext.New("user"), nil)
		require.Error(t, err)
	})

	t.Run("interpolate single kind context", func(t *testing.T) {
		context := ldcontext.NewBuilder("123").Name("Sandy").Build()
		result, err := eval(t, "I'm a {{ ldctx.kind}} with key {{ ldctx.key }}, named {{ ldctx.name }}", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "I'm a user with key 123, named Sandy", result)
	})

	t.Run("interpolation with nested context attributes", func(t *testing.T) {
		context := ldcontext.NewBuilder("123").
			SetValue("stats", ldvalue.ObjectBuild().Set("power", ldvalue.Int(9000)).Build()).Build()
		result, err := eval(t, "I can ingest over {{ ldctx.stats.power }} tokens per second!", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "I can ingest over 9000 tokens per second!", result)
	})

	t.Run("interpolation with multi kind context", func(t *testing.T) {
		user := ldcontext.NewBuilder("123").
			SetValue("cat_ownership", ldvalue.ObjectBuild().Set("count", ldvalue.Int(12)).Build()).Build()

		cat := ldcontext.NewBuilder("456").Kind("cat").
			SetValue("health", ldvalue.ObjectBuild().Set("hunger", ldvalue.String("off the charts")).Build()).Build()

		context := ldcontext.NewMulti(user, cat)

		result, err := eval(t, "As an owner of {{ ldctx.user.cat_ownership.count }} cats, I must report that my cat's hunger level is {{ ldctx.cat.health.hunger }}!", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "As an owner of 12 cats, I must report that my cat's hunger level is off the charts!", result)
	})

	t.Run("interpolation with multi kind context does not have anonymous attribute", func(t *testing.T) {
		user := ldcontext.NewBuilder("123").
			SetValue("cat_ownership", ldvalue.ObjectBuild().Set("count", ldvalue.Int(12)).Build()).Build()

		cat := ldcontext.NewBuilder("456").Kind("cat").
			SetValue("health", ldvalue.ObjectBuild().Set("hunger", ldvalue.String("off the charts")).Build()).Build()

		context := ldcontext.NewMulti(user, cat)

		result, err := eval(t, "anonymous=<{{ ldctx.anonymous }}>", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "anonymous=<>", result)
	})

	t.Run("interpolation with multi kind context has kind multi", func(t *testing.T) {
		user := ldcontext.NewBuilder("123").
			SetValue("cat_ownership", ldvalue.ObjectBuild().Set("count", ldvalue.Int(12)).Build()).Build()

		cat := ldcontext.NewBuilder("456").Kind("cat").
			SetValue("health", ldvalue.ObjectBuild().Set("hunger", ldvalue.String("off the charts")).Build()).Build()

		context := ldcontext.NewMulti(user, cat)

		result, err := eval(t, "kind=<{{ ldctx.kind }}>", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "kind=<multi>", result)
	})

	t.Run("interpolation with multi kind context does not have child kinds", func(t *testing.T) {

		// The idea here is that in a multi-kind context, we can access ldctx.kind (== "multi"), but you can't
		// access the kind field of the individual nested contexts since this doesn't match the actual data model.
		// That is, you can't access ldctx.user.kind or ldctx.cat.kind, only ldctx.kind.

		user := ldcontext.NewBuilder("123").
			SetValue("cat_ownership", ldvalue.ObjectBuild().Set("count", ldvalue.Int(12)).Build()).Build()

		cat := ldcontext.NewBuilder("456").Kind("cat").
			SetValue("health", ldvalue.ObjectBuild().Set("hunger", ldvalue.String("off the charts")).Build()).Build()

		context := ldcontext.NewMulti(user, cat)

		result, err := eval(t, "user_kind=<{{ ldctx.user.kind}}>,cat_kind=<{{ ldctx.cat.kind }}>", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "user_kind=<>,cat_kind=<>", result)
	})
}

func TestParseJudgeSpecificFields(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKey": "toxicity",
		"judgeConfiguration": {
			"judges": [
				{"key": "judge1", "samplingRate": 0.5},
				{"key": "judge2", "samplingRate": 1.0}
			]
		},
		"messages": [
			{"content": "test", "role": "system"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	assert.Equal(t, "judge", cfg.Mode())
	assert.Equal(t, "toxicity", cfg.EvaluationMetricKey())

	judgeConfig := cfg.JudgeConfiguration()
	require.NotNil(t, judgeConfig)
	require.Len(t, judgeConfig.Judges, 2)
	assert.Equal(t, "judge1", judgeConfig.Judges[0].Key)
	assert.Equal(t, 0.5, judgeConfig.Judges[0].SamplingRate)
	assert.Equal(t, "judge2", judgeConfig.Judges[1].Key)
	assert.Equal(t, 1.0, judgeConfig.Judges[1].SamplingRate)
}

func TestParseEvaluationMetricKeys(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKeys": ["relevance", "accuracy"],
		"messages": [
			{"content": "test", "role": "system"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	assert.Equal(t, "judge", cfg.Mode())
	assert.Equal(t, "", cfg.EvaluationMetricKey())
	assert.Equal(t, []string{"relevance", "accuracy"}, cfg.EvaluationMetricKeys())
}

func TestParseEvaluationMetricKeyPriority(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKey": "toxicity",
		"evaluationMetricKeys": ["relevance", "accuracy"],
		"messages": [
			{"content": "test", "role": "system"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	assert.Equal(t, "judge", cfg.Mode())
	// Both fields should be parsed
	assert.Equal(t, "toxicity", cfg.EvaluationMetricKey())
	assert.Equal(t, []string{"relevance", "accuracy"}, cfg.EvaluationMetricKeys())
}

func TestJudgeConfigurationImmutable(t *testing.T) {
	// Test that mutations to JudgeConfiguration don't affect the Config
	judgeConfig := &datamodel.JudgeConfiguration{
		Judges: []datamodel.Judge{
			{Key: "judge1", SamplingRate: 0.5},
			{Key: "judge2", SamplingRate: 1.0},
		},
	}

	builder := NewConfig().
		Enable().
		WithJudgeConfiguration(judgeConfig)
	cfg := builder.Build()

	// Mutate the original
	judgeConfig.Judges[0].Key = "mutated"
	judgeConfig.Judges = append(judgeConfig.Judges, datamodel.Judge{Key: "judge3", SamplingRate: 0.3})

	// Config should not be affected
	retrieved := cfg.JudgeConfiguration()
	require.NotNil(t, retrieved)
	require.Len(t, retrieved.Judges, 2)
	assert.Equal(t, "judge1", retrieved.Judges[0].Key) // Should still be original value
	assert.Equal(t, "judge2", retrieved.Judges[1].Key)

	// Mutate the retrieved config
	retrieved.Judges[0].Key = "mutated_again"
	retrieved.Judges = append(retrieved.Judges, datamodel.Judge{Key: "judge4", SamplingRate: 0.4})

	// Config should still not be affected
	retrieved2 := cfg.JudgeConfiguration()
	require.NotNil(t, retrieved2)
	require.Len(t, retrieved2.Judges, 2)
	assert.Equal(t, "judge1", retrieved2.Judges[0].Key) // Should still be original value
	assert.Equal(t, "judge2", retrieved2.Judges[1].Key)
}

// TestJudgeConfig_PreservesReservedPlaceholders verifies that JudgeConfig injects reserved variables
// so that {{message_history}} and {{response_to_evaluate}} are preserved for the second interpolation
// pass during Judge.Evaluate(). Without this, Config's first Mustache pass would render them as empty.
func TestJudgeConfig_PreservesReservedPlaceholders(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKey": "toxicity",
		"messages": [
			{"content": "You are a judge.", "role": "system"},
			{"content": "Input: {{message_history}}\nOutput: {{response_to_evaluate}}", "role": "user"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.JudgeConfig("judge-key", ldcontext.New("user"), Disabled(), nil)

	msgs := cfg.Messages()
	require.Len(t, msgs, 2)
	assert.Equal(t, "You are a judge.", msgs[0].Content)
	assert.Contains(t, msgs[1].Content, "{{message_history}}", "JudgeConfig must preserve placeholder for second interpolation")
	assert.Contains(t, msgs[1].Content, "{{response_to_evaluate}}", "JudgeConfig must preserve placeholder for second interpolation")
	assert.Equal(t, "Input: {{message_history}}\nOutput: {{response_to_evaluate}}", msgs[1].Content)
}

// TestConfig_WithoutReservedVarsWipesJudgePlaceholders documents that Config (without reserved vars)
// renders {{message_history}} and {{response_to_evaluate}} as empty when used for judge templates.
func TestConfig_WithoutReservedVarsWipesJudgePlaceholders(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"messages": [
			{"content": "Input: {{message_history}}\nOutput: {{response_to_evaluate}}", "role": "user"}
		]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	msgs := cfg.Messages()
	require.Len(t, msgs, 1)
	assert.Equal(t, "Input: \nOutput: ", msgs[0].Content, "Config without reserved vars renders placeholders as empty")
}

func TestCreateTracker_ManuallyBuiltConfig_ReturnsNil(t *testing.T) {
	cfg := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()
	assert.Nil(t, cfg.CreateTracker(), "manually built config should not have a tracker factory")
}

func TestCreateTracker_DisabledConfig_ReturnsTracker(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": false},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)
	assert.False(t, cfg.Enabled())
	assert.NotNil(t, cfg.CreateTracker(), "disabled config should still have a tracker factory")
}

func TestCreateTracker_EnabledConfig_ReturnsTracker(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"model": {"name": "gpt-4"},
		"provider": {"name": "openai"},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)
	assert.True(t, cfg.Enabled())

	tracker := cfg.CreateTracker()
	require.NotNil(t, tracker, "enabled config should have a tracker factory")
}

func TestCreateTracker_FreshRunIdPerCall(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	mockSDK := newMockSDK(json, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)

	// Clear SDK info event
	mockSDK.events = nil

	cfg := client.CompletionConfig("key", ldcontext.New("user"), Disabled(), nil)

	tracker1 := cfg.CreateTracker()
	tracker2 := cfg.CreateTracker()
	require.NotNil(t, tracker1)
	require.NotNil(t, tracker2)

	// Each tracker should be able to track independently. Track success on both to emit events.
	_ = tracker1.TrackSuccess()
	_ = tracker2.TrackSuccess()

	// Filter out the usage event; we only want the generation events.
	var genEvents []mockEvent
	for _, e := range mockSDK.events {
		if e.eventName == "$ld:ai:generation:success" {
			genEvents = append(genEvents, e)
		}
	}

	require.Len(t, genEvents, 2, "each tracker should emit its own event")

	runId1 := genEvents[0].data.GetByKey("runId").StringValue()
	runId2 := genEvents[1].data.GetByKey("runId").StringValue()
	assert.NotEmpty(t, runId1)
	assert.NotEmpty(t, runId2)
	assert.NotEqual(t, runId1, runId2, "each tracker must have a unique runId")
}

func TestCreateTracker_TrackerHasCorrectMetadata(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "var-1", "enabled": true, "version": 5},
		"model": {"name": "gpt-4"},
		"provider": {"name": "openai"},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	mockSDK := newMockSDK(json, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)

	// Clear SDK info event
	mockSDK.events = nil

	cfg := client.CompletionConfig("my-config", ldcontext.New("user"), Disabled(), nil)

	tracker := cfg.CreateTracker()
	require.NotNil(t, tracker)

	_ = tracker.TrackSuccess()

	// Filter for the generation event (skip usage event)
	var genEvent *mockEvent
	for i, e := range mockSDK.events {
		if e.eventName == "$ld:ai:generation:success" {
			genEvent = &mockSDK.events[i]
			break
		}
	}
	require.NotNil(t, genEvent)

	data := genEvent.data
	assert.Equal(t, "my-config", data.GetByKey("configKey").StringValue())
	assert.Equal(t, "var-1", data.GetByKey("variationKey").StringValue())
	assert.Equal(t, 5, data.GetByKey("version").IntValue())
	assert.Equal(t, "openai", data.GetByKey("providerName").StringValue())
	assert.Equal(t, "gpt-4", data.GetByKey("modelName").StringValue())
	assert.NotEmpty(t, data.GetByKey("runId").StringValue())
}

func TestCreateTracker_JudgeConfigHasFactory(t *testing.T) {
	json := []byte(`{
		"_ldMeta": {"variationKey": "1", "enabled": true},
		"mode": "judge",
		"evaluationMetricKey": "toxicity",
		"messages": [{"content": "test", "role": "system"}]
	}`)

	client, err := NewClient(newMockSDK(json, nil))
	require.NoError(t, err)

	cfg := client.JudgeConfig("judge-key", ldcontext.New("user"), Disabled(), nil)
	assert.True(t, cfg.Enabled())

	tracker := cfg.CreateTracker()
	require.NotNil(t, tracker, "enabled judge config should have a tracker factory")
}

func TestClient_CreateTracker_RoundTrip(t *testing.T) {
	configJSON := []byte(`{
		"_ldMeta": {"variationKey": "var-1", "enabled": true, "version": 5},
		"model": {"name": "gpt-4"},
		"provider": {"name": "openai"},
		"messages": [{"content": "hello", "role": "user"}]
	}`)

	mockSDK := newMockSDK(configJSON, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)

	// Clear SDK info event
	mockSDK.events = nil

	cfg := client.CompletionConfig("my-config", ldcontext.New("user"), Disabled(), nil)
	originalTracker := cfg.CreateTracker()
	require.NotNil(t, originalTracker)

	token := originalTracker.ResumptionToken()
	require.NotEmpty(t, token)

	// Reconstruct from token with a different context
	newContext := ldcontext.New("other-user")
	reconstructed, err := client.CreateTracker(token, newContext)
	require.NoError(t, err)
	require.NotNil(t, reconstructed)

	// The reconstructed tracker should produce the same resumption token
	assert.Equal(t, token, reconstructed.ResumptionToken())

	// Track feedback on the reconstructed tracker and verify it uses the original runId
	_ = originalTracker.TrackSuccess()
	_ = reconstructed.TrackFeedback(FeedbackPositive)

	var successEvent, feedbackEvent *mockEvent
	for i, e := range mockSDK.events {
		switch e.eventName {
		case "$ld:ai:generation:success":
			successEvent = &mockSDK.events[i]
		case "$ld:ai:feedback:user:positive":
			feedbackEvent = &mockSDK.events[i]
		}
	}
	require.NotNil(t, successEvent)
	require.NotNil(t, feedbackEvent)

	// Both events should share the same runId
	originalRunId := successEvent.data.GetByKey("runId").StringValue()
	reconstructedRunId := feedbackEvent.data.GetByKey("runId").StringValue()
	assert.Equal(t, originalRunId, reconstructedRunId, "reconstructed tracker must reuse the original runId")

	// Reconstructed tracker should use the new context
	assert.Equal(t, newContext, feedbackEvent.context)

	// Verify metadata preserved
	assert.Equal(t, "my-config", feedbackEvent.data.GetByKey("configKey").StringValue())
	assert.Equal(t, "var-1", feedbackEvent.data.GetByKey("variationKey").StringValue())
	assert.Equal(t, 5, feedbackEvent.data.GetByKey("version").IntValue())

	// modelName and providerName should be empty on reconstructed tracker
	assert.Equal(t, "", feedbackEvent.data.GetByKey("modelName").StringValue())
	assert.Equal(t, "", feedbackEvent.data.GetByKey("providerName").StringValue())
	assert.False(t, feedbackEvent.data.GetByKey("modelKey").IsDefined())
	assert.Equal(t, 1, feedbackEvent.data.GetByKey("modelVersion").IntValue())
}

func TestClient_CreateTracker_InvalidToken(t *testing.T) {
	mockSDK := newMockSDK(nil, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)

	t.Run("invalid base64", func(t *testing.T) {
		_, err := client.CreateTracker("not-valid-base64!!!", ldcontext.New("user"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resumption token")
	})

	t.Run("valid base64 but invalid JSON", func(t *testing.T) {
		token := base64.RawURLEncoding.EncodeToString([]byte("not json"))
		_, err := client.CreateTracker(token, ldcontext.New("user"))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid resumption token")
	})

	t.Run("valid token with missing fields uses zero values", func(t *testing.T) {
		payload, _ := json.Marshal(map[string]interface{}{"runId": "test-run"})
		token := base64.RawURLEncoding.EncodeToString(payload)
		tracker, err := client.CreateTracker(token, ldcontext.New("user"))
		require.NoError(t, err)
		require.NotNil(t, tracker)

		// Should work with partial data
		resumeToken := tracker.ResumptionToken()
		assert.NotEmpty(t, resumeToken)
	})
}
