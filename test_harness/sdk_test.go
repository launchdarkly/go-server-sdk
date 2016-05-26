package ldtest

import (
	"encoding/json"
	ld "github.com/launchdarkly/go-client"
	"log"
	"os"
	"testing"
	"time"
)

const (
	TestDataPath = "./testdata"
)

func TestPrerequisiteCycleDetection(t *testing.T) {
	allTestData, err := LoadTestData(TestDataPath)
	if err != nil {
		t.Fatalf("Error loading test data: %+v", err)
	}

	store := ld.NewInMemoryFeatureStore()
	var config = ld.Config{
		BaseUri:       "https://localhost:3000",
		Capacity:      1000,
		FlushInterval: 5 * time.Second,
		Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Timeout:       1500 * time.Millisecond,
		Stream:        true,
		FeatureStore:  store,
		Offline:       true,
	}
	client, err := ld.MakeCustomClient("api_key", config, 0)
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}

	for _, featureFlag := range allTestData.FeatureFlagsToCreate {
		err = store.Upsert(featureFlag.Key, featureFlag)
		if err != nil {
			t.Fatalf("Error upserting Feature Flag: %v", err)
		}
	}

	for _, s := range allTestData.Scenarios {
		t.Log("")
		t.Logf("Evaluating scenario: %s using feature key: %s", s.Name, s.FeatureKey)
		t.Logf("\tFound %d test cases to evaluate", len(s.TestCases))
		for _, testCase := range s.TestCases {
			userOk := true
			result, err := client.Evaluate(s.FeatureKey, testCase.User, s.DefaultValue)
			if err != nil {
				if !testCase.ExpectError {
					userOk = false
					t.Errorf("\tERROR: Unexpected error: %+v", err)
				} else {
					t.Logf("\tGot Expected Error: %+v", err)
				}
			} else {
				if testCase.ExpectError {
					userOk = false
					t.Error("\tFAIL: Didn't get expected error")
				}
			}
			if result != testCase.ExpectedValue {
				userOk = false
				t.Errorf("\tFAIL: Expected value: %+v. Instead got: %+v", testCase.ExpectedValue, result)
			}
			if !userOk {
				user, _ := json.Marshal(testCase.User)
				t.Errorf("\t\tWhen evaluating user: %s", string(user))
			}
			t.Logf("\tOK")
		}
	}
}
