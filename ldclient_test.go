package ldclient

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldlog"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

type mockDataSource struct {
	IsInitialized bool
	CloseFn       func() error
	StartFn       func(chan<- struct{})
}

func (u mockDataSource) Initialized() bool {
	return u.IsInitialized
}

func (u mockDataSource) Close() error {
	if u.CloseFn == nil {
		return nil
	}
	return u.CloseFn()
}

func (u mockDataSource) Start(closeWhenReady chan<- struct{}) {
	if u.StartFn == nil {
		return
	}
	u.StartFn(closeWhenReady)
}

func dataSourceFactory(u interfaces.DataSource) func(string, Config) (interfaces.DataSource, error) {
	return func(key string, c Config) (interfaces.DataSource, error) {
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

	hash := client.SecureModeHash(lduser.NewUser(key))

	assert.Equal(t, expected, hash)
}

func TestIdentifySendsIdentifyEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	err := client.Identify(user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(IdentifyEvent)
	assert.Equal(t, user, e.User)
}

func TestIdentifyWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackEventSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, ldvalue.Null(), e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestTrackDataSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	err := client.TrackData(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 1, len(events))
	e := events[0].(CustomEvent)
	assert.Equal(t, user, e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.Nil(t, e.MetricValue)
}

func TestTrackMetricSendsCustomEventWithMetricAndData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
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
	assert.Equal(t, data, e.Data)
	assert.Equal(t, &metric, e.MetricValue)
}

func TestTrackWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackEvent("eventkey", lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestTrackMetricWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackMetric("eventKey", lduser.NewUser(""), 2.5, ldvalue.Null())
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*testEventProcessor).events
	assert.Equal(t, 0, len(events))
}

func TestMakeCustomClient_WithFailedInitialization(t *testing.T) {
	dataSource := mockDataSource{
		IsInitialized: false,
		StartFn: func(closeWhenReady chan<- struct{}) {
			close(closeWhenReady)
		},
	}

	client, err := MakeCustomClient("sdkKey", Config{
		Loggers:               ldlog.NewDisabledLoggers(),
		DataSourceFactory:     dataSourceFactory(dataSource),
		EventProcessor:        &testEventProcessor{},
		UserKeysFlushInterval: 30 * time.Second,
	}, time.Second)

	assert.NotNil(t, client)
	assert.Equal(t, err, ErrInitializationFailed)
}

func makeTestClient() *LDClient {
	return makeTestClientWithConfig(nil)
}

func makeTestClientWithConfig(modConfig func(*Config)) *LDClient {
	config := Config{
		Offline:          false,
		SendEvents:       true,
		DataStoreFactory: NewInMemoryDataStoreFactory(),
		DataSourceFactory: dataSourceFactory(mockDataSource{
			IsInitialized: true,
		}),
		EventProcessor:        &testEventProcessor{},
		UserKeysFlushInterval: 30 * time.Second,
	}
	config.Loggers.SetBaseLogger(newMockLogger(""))
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
