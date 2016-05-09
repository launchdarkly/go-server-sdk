package ldtest

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"
	ld "github.com/launchdarkly/go-client"
)

const (
	TestDataPath = "./testdata"
)

func TestSdk(t *testing.T) {
	filePaths, err := ReadTestDataDir(TestDataPath)
	if err != nil {
		t.Fatalf("Error loading test data: %+v", err)
	}

	for _, filePath := range filePaths {
		container, err := LoadTestDataFile(filePath)
		if err != nil {
			t.Fatalf("Error loading test data: %+v", err)
		}
		count := len(container)
		_, fileName := filepath.Split(filePath)

		for i, td := range container {
			pre := fmt.Sprintf("[%s %d/%d] ", fileName, i+1, count)
			t.Log("")
			t.Logf("%sEvaluating Feature Flag: %s", pre, td.Name)

			userCount := len(td.TestCases)
			if userCount == 0 {
				t.Errorf("%s\tERROR: Found zero users for evaluation", pre)
				continue
			}
			t.Logf("%s\tFound %d users to evaluate", pre, userCount)
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
				t.Fatalf("%s\tFATAL: Error creating client: %v", pre, err)
			}

			for _, featureFlag := range td.FeatureFlags {
				err = store.Upsert(featureFlag.Key, featureFlag)
				if err != nil {
					t.Fatalf("%s\tFATAL: Error upserting Feature Flag: %v", pre, err)
				}
			}

			for _, testCase := range td.TestCases {
				userOk := true
				result, err := client.Evaluate(td.FeatureKey, testCase.User, td.DefaultValue)
				if err != nil {
					if !testCase.ExpectError {
						userOk = false
						t.Errorf("%s\tERROR: Unexpected error: %+v", pre, err)
					} else {
						t.Logf("%s\tGot Expected Error: %+v", pre, err)
					}
				} else {
					if testCase.ExpectError {
						userOk = false
						t.Errorf("%s\tERROR: Didn't get expected error", pre)
					}
				}
				if result != testCase.ExpectedValue {
					userOk = false
					t.Errorf("%s\tERROR: Expected value: %+v. Instead got: %+v", pre, testCase.ExpectedValue, result)
				}
				if !userOk {
					user, _ := json.Marshal(testCase.User)
					t.Errorf("%s\t\tWhen evaluating user: %s", pre, string(user))
				}
			}
		}
	}
}

