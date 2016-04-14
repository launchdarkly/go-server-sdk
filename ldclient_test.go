package ldclient

import (
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

const (
	validFeatureKey   = "validFeatureKey1"
	invalidFeatureKey = "invalidFeatureKey1"
	fallThroughValue  = "FallthroughValue"
	defaultValue      = "DefaultValue"
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
	on            bool
	deleted       bool
	featureKey    string
	expectedValue interface{}
	expectError   bool
}

func TestEvaluate(t *testing.T) {
	testData := []evaluateTestData{
		evaluateTestData{
			on:            true,
			deleted:       false,
			featureKey:    validFeatureKey,
			expectedValue: fallThroughValue,
		},
		evaluateTestData{
			on:            false,
			deleted:       false,
			featureKey:    validFeatureKey,
			expectedValue: defaultValue,
		},
		evaluateTestData{
			on:            true,
			deleted:       true,
			featureKey:    validFeatureKey,
			expectedValue: defaultValue,
			expectError:   true,
		},
		evaluateTestData{
			on:            true,
			deleted:       false,
			featureKey:    invalidFeatureKey,
			expectedValue: defaultValue,
			expectError:   true,
		},
		evaluateTestData{
			on:            false,
			deleted:       false,
			featureKey:    invalidFeatureKey,
			expectedValue: defaultValue,
			expectError:   true,
		},
	}

	for _, td := range testData {
		t.Logf("Testing evaluate with: On: %v, Deleted: %v, Feature Key: %v, Expected Value: %v, Expect Error? %v", td.on, td.deleted, td.featureKey, td.expectedValue, td.expectError)
		client, _ = MakeCustomClient("api_key", config, 0)
		upsertFeatureFlag(td)
		userKey := "userKey"
		result, err := client.evaluate(td.featureKey, User{Key: &userKey}, defaultValue)

		if err != nil {
			if td.expectError {
				t.Logf("\tGot Expected error: %+v", err)
			} else {
				t.Errorf("\tUnexpected error: %+v", err)
			}
		} else {
			if td.expectError {
				t.Errorf("\tDidn't get expected error")
			}
		}

		if result != td.expectedValue {
			t.Errorf("\tExpected value: %+v. Instead got: %+v", td.expectedValue, result)
		}
	}
}

func upsertFeatureFlag(etd evaluateTestData) {
	fallThroughVariation := 1
	fallthroughRule := Rule{[]Clause{}, &fallThroughVariation, nil}
	variations := []interface{}{"You shouldn't get this", fallThroughValue}

	client.store.Upsert(validFeatureKey, FeatureFlag{
		Key:          validFeatureKey,
		On:           etd.on,
		Targets:      []Target{},
		Rules:        []Rule{},
		Fallthrough:  fallthroughRule,
		OffVariation: nil,
		Variations:   variations,
		Deleted:      etd.deleted,
	})
}
