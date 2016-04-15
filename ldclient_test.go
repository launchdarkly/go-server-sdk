package ldclient

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"
)

var (
	config = Config{
		BaseUri:       "https://localhost:3000",
		Capacity:      1000,
		FlushInterval: 5 * time.Second,
		Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Timeout:       1500 * time.Millisecond,
		Stream:        true,
		Offline:       true,
	}
	client *LDClient
)

func TestOfflineModeAlwaysReturnsDefaultValue(t *testing.T) {
	client, _ := MakeCustomClient("api_key", config, 0)
	var key = "foo"
	res, err := client.Toggle("anything", User{Key: &key}, true)

	if err != nil {
		t.Errorf("Unexpected error in Toggle: %+v", err)
	}

	if !res {
		t.Errorf("Offline mode should return default value, but doesn't")
	}
}

type evaluateTestData struct {
	FeatureKey    string      `json:"featureKey"`
	DefaultValue  string      `json:"defaultValue"`
	ExpectedValue string      `json:"expectedValue"`
	ExpectError   bool        `json:"expectError"`
	User          User        `json:"user"`
	FeatureFlag   FeatureFlag `json:"featureFlag"`
}

func TestEvaluate(t *testing.T) {
	var container []evaluateTestData
	file, err := ioutil.ReadFile("./test_data.json")
	if err != nil {
		t.Errorf("Error loading test_data.json file: %v\n", err)
		return
	}
	err = json.Unmarshal(file, &container)
	if err != nil {
		t.Errorf("Error unmarshalling test_data.json file: %v\n", err)
		return
	}

	for _, td := range container {
		json, _ := json.MarshalIndent(td, "", "  ")
		t.Logf("Test data: %s", string(json))
		client, _ = MakeCustomClient("api_key", config, 0)
		client.store.Upsert(td.FeatureFlag.Key, td.FeatureFlag)
		result, err := client.evaluate(td.FeatureKey, User{Key: td.User.Key}, td.DefaultValue)

		if err != nil {
			if td.ExpectError {
				t.Logf("\tGot Expected error: %+v", err)
			} else {
				t.Errorf("\tUnexpected error: %+v", err)
			}
		} else {
			if td.ExpectError {
				t.Errorf("\tDidn't get expected error")
			}
		}

		if result != td.ExpectedValue {
			t.Errorf("\tExpected value: %+v. Instead got: %+v", td.ExpectedValue, result)
		}
	}
}
