package ldai

import (
	"errors"
	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/ldai/internal/datamodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

type mockServerSDK struct {
	log  *ldlogtest.MockLog
	json []byte
	err  error
}

func newMockSDK(json []byte, err error) *mockServerSDK {
	return &mockServerSDK{json: json, err: err, log: ldlogtest.NewMockLog()}
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

func TestInvalidConfigReturnsDefault(t *testing.T) {
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

			sdk.log.AssertMessageMatch(t, true, ldlog.Warn, "AI config 'key':")
		})
	}
}

func TestDisabledConfigs(t *testing.T) {
	tests := []struct {
		name string
		json []byte
	}{
		{"empty object", []byte("{}")},
		{"missing meta field", []byte(`{"model": {}, "messages": []}`)},
		{"meta disabled explicitly", []byte(`{"meta": {"enabled": false, "versionKey": "1"}, "model": {}, "messages": []}`)},
		{"meta disable implicitly", []byte(`{"meta": { "versionKey": "1"}, "model": {}, "messages": []}`)},
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

func TestCanSetDefaultConfigFields(t *testing.T) {
	client, err := NewClient(newMockSDK(nil, nil))
	require.NoError(t, err)
	require.NotNil(t, client)

	defaultVal := NewConfig().Enable().
		WithMessage("hello", datamodel.User).
		WithMessage("world", datamodel.System).
		WithProviderId("provider").
		WithModelId("model").Build()

	cfg, _ := client.Config("key", ldcontext.New("user"), defaultVal, nil)

	assert.True(t, cfg.Enabled())
	assert.Equal(t, "provider", cfg.ProviderId())
	assert.Equal(t, "model", cfg.ModelId())
	assert.Equal(t, 2, len(cfg.Messages()))

	msg := cfg.Messages()
	assert.Equal(t, "hello", msg[0].Content())
	assert.Equal(t, datamodel.User, msg[0].Role())
	assert.Equal(t, "world", msg[1].Content())
	assert.Equal(t, datamodel.System, msg[1].Role())
}

func eval(t *testing.T, prompt string, ctx ldcontext.Context, variables map[string]interface{}) (string, error) {
	t.Helper()
	json := []byte(`{
					"_ldMeta": {"versionKey": "1", "enabled": true},
					"model": {},
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
	return cfg.Messages()[0].Content(), nil
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

		result, err := eval(t, "kind={{ ldctx.kind }}", context, nil)
		require.NoError(t, err)
		assert.Equal(t, "kind=multi", result)
	})
}
