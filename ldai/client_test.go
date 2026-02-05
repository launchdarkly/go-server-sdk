package ldai

import (
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
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)
}

func TestEvalErrorReturnsDefault(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, errors.New("client is offline")))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().Enable().WithMessage("hello", datamodel.User).Build()

	cfg, tracker := client.Config("key", ldcontext.New("user"), defaultVal, nil)
	assert.NotNil(t, tracker)
	assert.Equal(t, defaultVal, cfg)
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

	cfg, _ := client.Config("key", ldcontext.New("user"), Disabled(), nil)

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
			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

			assert.Equal(t, test.expected, cfg.ModelName())
		})
	}
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
			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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

			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)
			assert.Equal(t, defaultVal, cfg)

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

			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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
			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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
			cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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

	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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

func TestConfigMethodTracking(t *testing.T) {
	mockSDK := newMockSDK(nil, nil)
	client, err := NewClient(mockSDK)
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultConfig := NewConfig().WithEnabled(false).Build()
	context := ldcontext.New("user-key")
	configKey := "test-config-key"

	config, tracker := client.Config(configKey, context, defaultConfig, nil)

	require.NotNil(t, config)
	require.NotNil(t, tracker)

	expectedEvents := []mockEvent{
		{
			eventName:   "$ld:ai:config:function:single",
			context:     context,
			metricValue: 1,
			data:        ldvalue.String(configKey),
		},
	}

	assert.ElementsMatch(t, expectedEvents, mockSDK.events)
}

func TestCanSetModelParameters(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().WithModelParam("foo", ldvalue.String("bar")).Build()
	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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
	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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

	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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

	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

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
	cfg, _ := client.Config("key", ctx, Disabled(), variables)
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

	cfg, _ := client.Config("key", ldcontext.New("user"), Disabled(), nil)

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

	cfg, _ := client.Config("key", ldcontext.New("user"), Disabled(), nil)

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

	cfg, _ := client.Config("key", ldcontext.New("user"), Disabled(), nil)

	assert.Equal(t, "judge", cfg.Mode())
	// Both fields should be parsed
	assert.Equal(t, "toxicity", cfg.EvaluationMetricKey())
	assert.Equal(t, []string{"relevance", "accuracy"}, cfg.EvaluationMetricKeys())
}

func TestJudgeConfigurationImmutable(t *testing.T) {
	// Test that mutations to JudgeConfiguration don't affect the Config
	judgeConfig := &datamodel.JudgeConfiguration{
		Judges: []datamodel.LDJudge{
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
	judgeConfig.Judges = append(judgeConfig.Judges, datamodel.LDJudge{Key: "judge3", SamplingRate: 0.3})

	// Config should not be affected
	retrieved := cfg.JudgeConfiguration()
	require.NotNil(t, retrieved)
	require.Len(t, retrieved.Judges, 2)
	assert.Equal(t, "judge1", retrieved.Judges[0].Key) // Should still be original value
	assert.Equal(t, "judge2", retrieved.Judges[1].Key)

	// Mutate the retrieved config
	retrieved.Judges[0].Key = "mutated_again"
	retrieved.Judges = append(retrieved.Judges, datamodel.LDJudge{Key: "judge4", SamplingRate: 0.4})

	// Config should still not be affected
	retrieved2 := cfg.JudgeConfiguration()
	require.NotNil(t, retrieved2)
	require.Len(t, retrieved2.Judges, 2)
	assert.Equal(t, "judge1", retrieved2.Judges[0].Key) // Should still be original value
	assert.Equal(t, "judge2", retrieved2.Judges[1].Key)
}
