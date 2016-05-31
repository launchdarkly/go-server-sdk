package ldclient

import (
	"log"
	"os"
	"testing"
	"time"
)

func TestPrereqSelfCycle(t *testing.T) {
	featureFlagA := newFlagWithPrereq("keyA", "keyA")
	testFlag(t, "key", []FeatureFlag{featureFlagA})
}

func TestPrereqSimpleCycle(t *testing.T) {
	featureFlagA := newFlagWithPrereq("keyA", "keyB")
	featureFlagB := newFlagWithPrereq("keyB", "keyA")
	testFlag(t, "keyA", []FeatureFlag{featureFlagA, featureFlagB})
}

func TestPrereqCycle(t *testing.T) {
	featureFlagA := newFlagWithPrereq("keyA", "keyB")
	featureFlagB := newFlagWithPrereq("keyB", "keyC")
	featureFlagC := newFlagWithPrereq("keyC", "keyA")
	testFlag(t, "keyA", []FeatureFlag{featureFlagA, featureFlagB, featureFlagC})
}

func testFlag(t *testing.T, key string, flags []FeatureFlag) {
	store := NewInMemoryFeatureStore()
	var config = Config{
		BaseUri:       "https://localhost:3000",
		Capacity:      1000,
		FlushInterval: 5 * time.Second,
		Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Timeout:       1500 * time.Millisecond,
		Stream:        true,
		FeatureStore:  store,
		Offline:       true,
	}
	client, err := MakeCustomClient("api_key", config, 0)
	if err != nil {
		t.Fatalf("Error creating client: %v", err)
	}
	for _, flag := range flags {
		err = store.Upsert(flag.Key, flag)
		if err != nil {
			t.Fatalf("Error upserting Feature Flag: %v", err)
		}
	}
	userKey := "userKey"
	user := User{Key: &userKey}

	result, _ := client.Evaluate(key, user, "defaultValue")
	if result != "defaultValue" {
		t.Errorf("\tFAIL: Expected value: defaultValue. Instead got: %+v", result)
	}
}

func newFlagWithPrereq(key string, prereq string) FeatureFlag {
	fallthroughVariation := 0
	return FeatureFlag{
		Key:           key,
		On:            true,
		Prerequisites: []Prerequisite{Prerequisite{Key: prereq, Variation: 0}},
		Fallthrough:   Rule{Variation: &fallthroughVariation},
		Variations:    []interface{}{"a", "b"},
	}
}
