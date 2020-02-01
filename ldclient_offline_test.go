package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

func makeOfflineClient() *LDClient {
	config := Config{
		BaseUri:       "https://localhost:3000",
		Capacity:      1000,
		FlushInterval: 5 * time.Second,
		Timeout:       1500 * time.Millisecond,
		Stream:        true,
		Offline:       true,
	}
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
	assert.Equal(t, defaultVal, value)
	assert.Equal(t, defaultVal, detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
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
	assert.Equal(t, float64(defaultVal), detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
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
	assert.Equal(t, defaultVal, detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
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
	assert.Equal(t, defaultVal, detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
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
	assert.Equal(t, defaultVal, detail.JSONValue)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
}

func TestAllFlagsStateReturnsEmptyStateOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	result := client.AllFlagsState(evalTestUser)
	assert.False(t, result.IsValid())
}
