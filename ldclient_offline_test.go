package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"
)

func makeOfflineClient() *LDClient {
	config := Config{Offline: true, Loggers: sharedtest.NewTestLoggers()}
	client, _ := MakeCustomClient("api_key", config, 0)
	return client
}

func TestBoolVariationReturnsDefaultValueOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	defaultVal := true
	value, err := client.BoolVariation("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)

	value, detail, err := client.BoolVariationDetail("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, newEvaluationError(ldvalue.Bool(defaultVal), ldreason.EvalErrorClientNotReady), detail)
}

func TestIntVariationReturnsDefaultValueOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	defaultVal := 100
	value, err := client.IntVariation("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)

	value, detail, err := client.IntVariationDetail("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)
	assert.Equal(t, newEvaluationError(ldvalue.Int(defaultVal), ldreason.EvalErrorClientNotReady), detail)
}

func TestFloat64VariationReturnsDefaultValueOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	defaultVal := 100.0
	value, err := client.Float64Variation("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)

	value, detail, err := client.Float64VariationDetail("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)
	assert.Equal(t, newEvaluationError(ldvalue.Float64(defaultVal), ldreason.EvalErrorClientNotReady), detail)
}

func TestStringVariationReturnsDefaultValueOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	defaultVal := "expected"
	value, err := client.StringVariation("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)

	value, detail, err := client.StringVariationDetail("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)
	assert.Equal(t, newEvaluationError(ldvalue.String(defaultVal), ldreason.EvalErrorClientNotReady), detail)
}

func TestJsonVariationReturnsDefaultValueOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	defaultVal := ldvalue.ObjectBuild().Set("field2", ldvalue.String("value2")).Build()
	value, err := client.JSONVariation("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)

	value, detail, err := client.JSONVariationDetail("featureKey", evalTestUser, defaultVal)
	assert.NoError(t, err)
	assert.Equal(t, defaultVal, value)
	assert.Equal(t, newEvaluationError(defaultVal, ldreason.EvalErrorClientNotReady), detail)
}

func TestAllFlagsStateReturnsEmptyStateOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	result := client.AllFlagsState(evalTestUser)
	assert.False(t, result.IsValid())
}
