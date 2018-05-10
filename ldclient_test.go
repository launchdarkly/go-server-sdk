package ldclient

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testUpdateProcessor struct{}

func (u testUpdateProcessor) Initialized() bool     { return true }
func (u testUpdateProcessor) Close() error          { return nil }
func (u testUpdateProcessor) Start(chan<- struct{}) {}

type testEventProcessor struct {
	events []Event
}

func (t *testEventProcessor) SendEvent(e Event) {
	t.events = append(t.events, e)
}

func (t *testEventProcessor) Flush() {}

func (t *testEventProcessor) Close() error {
	return nil
}

func TestOfflineModeAlwaysReturnsDefaultValue(t *testing.T) {
	config := Config{
		BaseUri:       "https://localhost:3000",
		Capacity:      1000,
		FlushInterval: 5 * time.Second,
		Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Timeout:       1500 * time.Millisecond,
		Stream:        true,
		Offline:       true,
	}
	client, _ := MakeCustomClient("api_key", config, 0)
	defer client.Close()

	user := NewUser("foo")

	//BoolVariation
	actual, err := client.BoolVariation("featureKey", user, true)
	assert.NoError(t, err)
	assert.True(t, actual)

	//IntVariation
	expectedInt := 100
	actualInt, err := client.IntVariation("featureKey", user, expectedInt)
	assert.NoError(t, err)
	assert.Equal(t, expectedInt, actualInt)

	//Float64Variation
	expectedFloat64 := 100.0
	actualFloat64, err := client.Float64Variation("featureKey", user, expectedFloat64)
	assert.NoError(t, err)
	assert.Equal(t, expectedFloat64, actualFloat64)

	//StringVariation
	expectedString := "expected"
	actualString, err := client.StringVariation("featureKey", user, expectedString)
	assert.NoError(t, err)
	assert.Equal(t, expectedString, actualString)

	//JsonVariation
	expectedJsonString := `{"fieldName":"fieldValue"}`
	expectedJson := json.RawMessage([]byte(expectedJsonString))
	actualJson, err := client.JsonVariation("featureKey", user, expectedJson)
	assert.NoError(t, err)
	assert.Equal(t, string([]byte(expectedJson)), string([]byte(actualJson)))

	client.Close()
}

func TestBoolVariation(t *testing.T) {
	expected := true
	variations := []interface{}{false, true}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, featureFlagWithVariations("validFeatureKey", variations))

	actual, err := client.BoolVariation("validFeatureKey", NewUser("userKey"), false)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestIntVariation(t *testing.T) {
	expected := float64(100)

	variations := []interface{}{float64(-1), expected}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, featureFlagWithVariations("validFeatureKey", variations))

	actual, err := client.IntVariation("validFeatureKey", NewUser("userKey"), 10000)

	assert.NoError(t, err)
	assert.Equal(t, int(expected), actual)
}

func TestFloat64Variation(t *testing.T) {
	expected := 100.01

	variations := []interface{}{-1.0, expected}

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, featureFlagWithVariations("validFeatureKey", variations))

	actual, err := client.Float64Variation("validFeatureKey", NewUser("userKey"), 0.0)

	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestJsonVariation(t *testing.T) {
	expectedJsonString := `{"jsonFieldName2":"fallthroughValue"}`

	var variations []interface{}
	json.Unmarshal([]byte(fmt.Sprintf(`[{"jsonFieldName1" : "jsonFieldValue"},%s]`, expectedJsonString)), &variations)

	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, featureFlagWithVariations("validFeatureKey", variations))

	var actual json.RawMessage
	actual, err := client.JsonVariation("validFeatureKey", NewUser("userKey"), []byte(`{"default":"default"}`))

	assert.NoError(t, err)
	assert.Equal(t, expectedJsonString, string(actual))
}

func TestSecureModeHash(t *testing.T) {
	expected := "aa747c502a898200f9e4fa21bac68136f886a0e27aec70ba06daf2e2a5cb5597"
	key := "Message"
	config := DefaultConfig
	config.Offline = true

	client, _ := MakeCustomClient("secret", config, 0*time.Second)

	hash := client.SecureModeHash(User{Key: &key})

	assert.Equal(t, expected, hash)
}

func TestEvaluatingExistingFlagSendsEvent(t *testing.T) {
	flag := featureFlagWithVariations("flagKey", []interface{}{"a", "b"})
	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	user := NewUser("userKey")
	_, err := client.StringVariation(flag.Key, user, "x")
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events

	assert.Equal(t, 1, len(events))
	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         user,
		},
		Key:       flag.Key,
		Version:   &flag.Version,
		Value:     "b",
		Variation: intPtr(1),
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingUnknownFlagSendsEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	_, err := client.StringVariation("flagKey", user, "x")
	assert.Error(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))

	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         user,
		},
		Key:       "flagKey",
		Version:   nil,
		Value:     "x",
		Variation: nil,
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingFlagWithNilUserKeySendsEvent(t *testing.T) {
	flag := featureFlagWithVariations("flagKey", []interface{}{"a", "b"})
	client := makeTestClient()
	defer client.Close()
	client.store.Upsert(Features, flag)

	user := User{Name: strPtr("Bob")}
	_, err := client.StringVariation(flag.Key, user, "x")
	assert.Error(t, err)

	events := client.eventProcessor.(*testEventProcessor).events

	assert.Equal(t, 1, len(events))
	e := events[0].(FeatureRequestEvent)
	expectedEvent := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e.CreationDate,
			User:         user,
		},
		Key:       flag.Key,
		Version:   &flag.Version,
		Value:     "x",
		Variation: nil,
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expectedEvent, e)
}

func TestEvaluatingFlagWithPrerequisiteSendsPrerequisiteEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	flag0 := featureFlagWithVariations("flag0", []interface{}{"a", "b"})
	flag0.Prerequisites = []Prerequisite{
		Prerequisite{Key: "flag1", Variation: 1},
	}
	flag1 := featureFlagWithVariations("flag1", []interface{}{"c", "d"})
	client.store.Upsert(Features, flag0)
	client.store.Upsert(Features, flag1)

	user := NewUser("userKey")
	_, err := client.StringVariation(flag0.Key, user, "x")
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 2, len(events))

	e0 := events[0].(FeatureRequestEvent)
	expected0 := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e0.CreationDate,
			User:         user,
		},
		Key:       flag1.Key,
		Version:   &flag1.Version,
		Value:     "d",
		Variation: intPtr(1),
		Default:   nil,
		PrereqOf:  &flag0.Key,
	}
	assert.Equal(t, expected0, e0)

	e1 := events[1].(FeatureRequestEvent)
	expected1 := FeatureRequestEvent{
		BaseEvent: BaseEvent{
			CreationDate: e1.CreationDate,
			User:         user,
		},
		Key:       flag0.Key,
		Version:   &flag0.Version,
		Value:     "b",
		Variation: intPtr(1),
		Default:   "x",
		PrereqOf:  nil,
	}
	assert.Equal(t, expected1, e1)
}

func TestIdentifySendsIdentifyEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	err := client.Identify(user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(IdentifyEvent)
	assert.Equal(t, user, e.User)
}

func TestTrackSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	data := map[string]interface{}{"thing": "stuff"}
	err := client.Track(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
}

// Creates LdClient loaded with one feature flag with key: "validFeatureKey".
// Variations param should have at least 2 items with variations[1] being the expected
// fallthrough value when passing in a valid user
func makeTestClient() *LDClient {
	config := Config{
		Logger:                log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Offline:               false,
		SendEvents:            true,
		FeatureStore:          NewInMemoryFeatureStore(nil),
		UpdateProcessor:       testUpdateProcessor{},
		EventProcessor:        &testEventProcessor{},
		UserKeysFlushInterval: 30 * time.Second,
	}

	client, _ := MakeCustomClient("sdkKey", config, 0*time.Second)
	return client
}

func featureFlagWithVariations(key string, variations []interface{}) *FeatureFlag {
	fallThroughVariation := 1

	return &FeatureFlag{
		Key:         key,
		Version:     1,
		On:          true,
		Fallthrough: VariationOrRollout{Variation: &fallThroughVariation},
		Variations:  variations,
	}
}
