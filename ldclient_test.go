package ldclient

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v1/ldvalue"
	shared "gopkg.in/launchdarkly/go-server-sdk.v4/shared_test"
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

	hash := client.SecureModeHash(NewUser(key))

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

func TestIdentifyWithNilUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(evalTestUserWithNilKey)
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestIdentifyWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackEventSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Nil(t, e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestTrackDataSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	err := client.TrackData(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data.AsArbitraryValue(), e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestTrackMetricSendsCustomEventWithMetricAndData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	metric := float64(1.5)
	err := client.TrackMetric(key, user, metric, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data.AsArbitraryValue(), e.Data)
	assert.Equal(t, &metric, e.MetricValue)
}

func TestDeprecatedTrackSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	err := client.Track(key, user, nil) //nolint:megacheck // allow deprecated usage
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Nil(t, e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestDeprecatedTrackSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	data := map[string]interface{}{"thing": "stuff"}
	err := client.Track(key, user, data) //nolint:megacheck // allow deprecated usage
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestDeprecatedTrackWithMetricSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := NewUser("userKey")
	key := "eventKey"
	value := 2.5
	data := map[string]interface{}{"thing": "stuff"}
	err := client.TrackWithMetric(key, user, data, value) //nolint:megacheck // allow deprecated usage
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.Equal(t, value, *e.MetricValue)
}

func TestTrackWithNilUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Track("eventkey", evalTestUserWithNilKey, nil)
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Track("eventkey", NewUser(""), nil)
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackWithMetricWithNilUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	data := map[string]interface{}{"thing": "stuff"}
	err := client.TrackWithMetric("eventKey", evalTestUserWithNilKey, data, 2.5)
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackWithMetricWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	data := map[string]interface{}{"thing": "stuff"}
	err := client.TrackWithMetric("eventKey", NewUser(""), data, 2.5)
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestMakeCustomClient_WithFailedInitialization(t *testing.T) {
	updateProcessor := mockUpdateProcessor{
		IsInitialized: false,
		StartFn: func(closeWhenReady chan<- struct{}) {
			close(closeWhenReady)
		},
	}

	client, err := MakeCustomClient("sdkKey", Config{
		Loggers:                shared.NullLoggers(),
		UpdateProcessorFactory: updateProcessorFactory(updateProcessor),
		EventProcessor:         &testEventProcessor{},
		UserKeysFlushInterval:  30 * time.Second,
	}, time.Second)

	assert.NotNil(t, client)
	assert.Equal(t, err, ErrInitializationFailed)
}

func makeTestClient() *LDClient {
	return makeTestClientWithConfig(nil)
}

func makeTestClientWithConfig(modConfig func(*Config)) *LDClient {
	config := Config{
		Logger:       newMockLogger(""),
		Offline:      false,
		SendEvents:   true,
		FeatureStore: NewInMemoryFeatureStore(nil),
		UpdateProcessorFactory: updateProcessorFactory(mockUpdateProcessor{
			IsInitialized: true,
		}),
		EventProcessor:        &testEventProcessor{},
		UserKeysFlushInterval: 30 * time.Second,
	}
	if modConfig != nil {
		modConfig(&config)
	}
	client, _ := MakeCustomClient("sdkKey", config, time.Duration(0))
	return client
}

func newMockLogger(prefix string) *mockLogger {
	return &mockLogger{output: make([]string, 0), prefix: prefix}
}

type mockLogger struct {
	output []string
	prefix string
}

func (l *mockLogger) append(s string) {
	if l.prefix == "" || strings.HasPrefix(s, l.prefix) {
		l.output = append(l.output, s)
	}
}

func (l *mockLogger) Println(args ...interface{}) {
	l.append(strings.TrimSpace(fmt.Sprintln(args...)))
}

func (l *mockLogger) Printf(format string, args ...interface{}) {
	l.append(fmt.Sprintf(format, args...))
}
