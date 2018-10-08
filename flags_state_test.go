package ldclient

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFlagsStateCanGetFlagValue(t *testing.T) {
	flag := FeatureFlag{Key: "key"}
	state := newFeatureFlagsState()
	state.addFlag(&flag, "value", intPtr(1), nil, false)

	assert.Equal(t, "value", state.GetFlagValue("key"))
}

func TestFlagsStateUnknownFlagReturnsNilValue(t *testing.T) {
	state := newFeatureFlagsState()

	assert.Nil(t, state.GetFlagValue("key"))
}

func TestFlagsStateCanGetFlagReason(t *testing.T) {
	flag := FeatureFlag{Key: "key"}
	state := newFeatureFlagsState()
	state.addFlag(&flag, "value", intPtr(1), evalReasonOffInstance, false)

	assert.Equal(t, evalReasonOffInstance, state.GetFlagReason("key"))
}

func TestFlagsStateUnknownFlagReturnsNilReason(t *testing.T) {
	state := newFeatureFlagsState()

	assert.Nil(t, state.GetFlagReason("key"))
}

func TestFlagsStateReturnsNilReasonIfReasonsWereNotRecored(t *testing.T) {
	flag := FeatureFlag{Key: "key"}
	state := newFeatureFlagsState()
	state.addFlag(&flag, "value", intPtr(1), nil, false)

	assert.Nil(t, state.GetFlagReason("key"))
}

func TestFlagsStateToValuesMap(t *testing.T) {
	flag1 := FeatureFlag{Key: "key1"}
	flag2 := FeatureFlag{Key: "key2"}
	state := newFeatureFlagsState()
	state.addFlag(&flag1, "value1", intPtr(0), nil, false)
	state.addFlag(&flag2, "value2", intPtr(1), nil, false)

	expected := map[string]interface{}{"key1": "value1", "key2": "value2"}
	assert.Equal(t, expected, state.ToValuesMap())
}

func TestFlagsStateToJSON(t *testing.T) {
	date := uint64(1000)
	flag1 := FeatureFlag{Key: "key1", Version: 100, TrackEvents: false}
	flag2 := FeatureFlag{Key: "key2", Version: 200, TrackEvents: true, DebugEventsUntilDate: &date}
	state := newFeatureFlagsState()
	state.addFlag(&flag1, "value1", intPtr(0), nil, false)
	state.addFlag(&flag2, "value2", intPtr(1), nil, false)

	expectedString := `{
		"key1":"value1",
		"key2":"value2",
		"$flagsState":{
	  		"key1":{
				"variation":0,"version":100
			},
			"key2": {
				"variation":1,"version":200,"trackEvents":true,"debugEventsUntilDate":1000
			}
		},
		"$valid":true
	}`
	actualBytes, err := json.Marshal(state)
	assert.NoError(t, err)
	assert.JSONEq(t, expectedString, string(actualBytes))
}
