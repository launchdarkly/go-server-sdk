package ldclient

import (
	"log"
	"os"
	"testing"
	"time"
)

var config = Config{
	BaseUri:       "https://localhost:3000",
	Capacity:      1000,
	FlushInterval: 5 * time.Second,
	Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
	Timeout:       1500 * time.Millisecond,
}

func TestOfflineModeAlwaysReturnsDefaultValue(t *testing.T) {
	client := MakeCustomClient("api_key", config)
	client.SetOffline()
	var key = "foo"
	res, err := client.Toggle("anything", User{Key: &key}, true)

	if err != nil {
		t.Errorf("Unexpected error in Toggle")
	}

	if !res {
		t.Errorf("Offline mode should return default value, but doesn't")
	}
}
