package ldclient

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

	defaultVal := map[string]interface{}{"field2": "value2"}
	defaultJSON, _ := json.Marshal(defaultVal)
	value, err := client.JsonVariation("featureKey", evalTestUser, defaultJSON)
	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(defaultJSON), value)

	value, detail, err := client.JsonVariationDetail("featureKey", evalTestUser, defaultJSON)
	assert.NoError(t, err)
	assert.Equal(t, json.RawMessage(defaultJSON), value)
	assert.Equal(t, json.RawMessage(defaultJSON), detail.Value)
	assert.Nil(t, detail.VariationIndex)
	assert.Equal(t, newEvalReasonError(EvalErrorClientNotReady), detail.Reason)
}

func TestAllFlagsReturnsNilOffline(t *testing.T) {
	client := makeOfflineClient()
	defer client.Close()

	result := client.AllFlags(evalTestUser)
	assert.Nil(t, result)
}
