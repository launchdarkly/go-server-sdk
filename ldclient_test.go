package ldclient

import (
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockUpdateProcessor struct {
	IsInitialized bool
	CloseFn       func() error
	StartFn       func(chan<- struct{})
}

func (u mockUpdateProcessor) Initialized() bool {
	return u.IsInitialized
}

func (u mockUpdateProcessor) Close() error {
	if u.CloseFn == nil {
		return nil
	}
	return u.CloseFn()
}

func (u mockUpdateProcessor) Start(closeWhenReady chan<- struct{}) {
	if u.StartFn == nil {
		return
	}
	u.StartFn(closeWhenReady)
}

func updateProcessorFactory(u UpdateProcessor) func(string, Config) (UpdateProcessor, error) {
	return func(key string, c Config) (UpdateProcessor, error) {
		return u, nil
	}
}

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

func TestSecureModeHash(t *testing.T) {
	expected := "aa747c502a898200f9e4fa21bac68136f886a0e27aec70ba06daf2e2a5cb5597"
	key := "Message"
	config := DefaultConfig
	config.Offline = true

	client, _ := MakeCustomClient("secret", config, 0*time.Second)

	hash := client.SecureModeHash(User{Key: &key})

	assert.Equal(t, expected, hash)
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

func TestMakeCustomClient_WithFailedInitialization(t *testing.T) {
	updateProcessor := mockUpdateProcessor{
		IsInitialized: false,
		StartFn: func(closeWhenReady chan<- struct{}) {
			close(closeWhenReady)
		},
	}

	client, err := MakeCustomClient("sdkKey", Config{
		Logger:                 log.New(ioutil.Discard, "", 0),
		UpdateProcessorFactory: updateProcessorFactory(updateProcessor),
		EventProcessor:         &testEventProcessor{},
		UserKeysFlushInterval:  30 * time.Second,
	}, time.Second)

	assert.NotNil(t, client)
	assert.Equal(t, err, ErrInitializationFailed)
}

func makeTestClient() *LDClient {
	config := Config{
		Logger:       log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
		Offline:      false,
		SendEvents:   true,
		FeatureStore: NewInMemoryFeatureStore(nil),
		UpdateProcessorFactory: updateProcessorFactory(mockUpdateProcessor{
			IsInitialized: true,
		}),
		EventProcessor:        &testEventProcessor{},
		UserKeysFlushInterval: 30 * time.Second,
	}

	client, _ := MakeCustomClient("sdkKey", config, time.Duration(0))
	return client
}
