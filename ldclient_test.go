package ldclient

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSdkKey = "test-sdk-key"

type badFactory struct{ err error }

func (f badFactory) CreateDataSource(c interfaces.ClientContext, u interfaces.DataSourceUpdates) (interfaces.DataSource, error) {
	return nil, f.err
}

func (f badFactory) CreateDataStore(c interfaces.ClientContext, u interfaces.DataStoreUpdates) (interfaces.DataStore, error) {
	return nil, f.err
}

func (f badFactory) CreateEventProcessor(context interfaces.ClientContext) (ldevents.EventProcessor, error) {
	return nil, f.err
}

func (f badFactory) CreateHTTPConfiguration(context interfaces.BasicConfiguration) (interfaces.HTTPConfiguration, error) {
	return nil, f.err
}

func (f badFactory) CreateLoggingConfiguration(context interfaces.BasicConfiguration) (interfaces.LoggingConfiguration, error) {
	return nil, f.err
}

func TestErrorFromComponentFactoryStopsClientCreation(t *testing.T) {
	fakeError := errors.New("sorry")
	factory := badFactory{fakeError}

	doTest := func(name string, config Config) {
		t.Run(name, func(t *testing.T) {
			client, err := MakeCustomClient(testSdkKey, config, 0)
			assert.Nil(t, client)
			assert.Equal(t, fakeError, err)
		})
	}

	doTest("DataSource", Config{DataSource: factory})
	doTest("DataStore", Config{DataStore: factory})
	doTest("Events", Config{Events: factory})
	doTest("HTTP", Config{HTTP: factory})
	doTest("Logging", Config{Logging: factory})
}

func TestSecureModeHash(t *testing.T) {
	expected := "aa747c502a898200f9e4fa21bac68136f886a0e27aec70ba06daf2e2a5cb5597"
	key := "Message"
	config := Config{Offline: true}

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

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.IdentifyEvent)
	assert.Equal(t, ldevents.User(user), e.User)
}

func TestIdentifyWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.Identify(lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackEventSendsCustomEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	err := client.TrackEvent(key, user)
	assert.NoError(t, err)

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, ldvalue.Null(), e.Data)
	assert.False(t, e.HasMetric)
}

func TestTrackDataSendsCustomEventWithData(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	user := lduser.NewUser("userKey")
	key := "eventKey"
	data := ldvalue.ArrayOf(ldvalue.String("a"), ldvalue.String("b"))
	err := client.TrackData(key, user, data)
	assert.NoError(t, err)

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.False(t, e.HasMetric)
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

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 1, len(events))
	e := events[0].(ldevents.CustomEvent)
	assert.Equal(t, ldevents.User(user), e.User)
	assert.Equal(t, key, e.Key)
	assert.Equal(t, data, e.Data)
	assert.True(t, e.HasMetric)
	assert.Equal(t, metric, e.MetricValue)
}

func TestTrackWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackEvent("eventkey", lduser.NewUser(""))
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestTrackMetricWithEmptyUserKeySendsNoEvent(t *testing.T) {
	client := makeTestClient()
	defer client.Close()

	err := client.TrackMetric("eventKey", lduser.NewUser(""), 2.5, ldvalue.Null())
	assert.NoError(t, err) // we don't return an error for this, we just log it

	events := client.eventProcessor.(*sharedtest.CapturingEventProcessor).Events
	assert.Equal(t, 0, len(events))
}

func TestIdentifyWithEventsDisabledDoesNotCauseError(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	client := makeTestClientWithConfig(func(c *Config) {
		c.Events = ldcomponents.NoEvents()
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	require.NoError(t, client.Identify(lduser.NewUser("")))

	assert.Len(t, mockLog.GetOutput(ldlog.Warn), 0)
}

func TestTrackWithEventsDisabledDoesNotCauseError(t *testing.T) {
	mockLog := ldlogtest.NewMockLog()
	client := makeTestClientWithConfig(func(c *Config) {
		c.Events = ldcomponents.NoEvents()
		c.Logging = ldcomponents.Logging().Loggers(mockLog.Loggers)
	})
	defer client.Close()

	require.NoError(t, client.TrackEvent("eventkey", lduser.NewUser("")))
	require.NoError(t, client.TrackMetric("eventkey", lduser.NewUser(""), 0, ldvalue.Null()))

	assert.Len(t, mockLog.GetOutput(ldlog.Warn), 0)
}

func TestMakeCustomClient_WithFailedInitialization(t *testing.T) {
	dataSource := sharedtest.MockDataSource{Initialized: false}

	client, err := MakeCustomClient(testSdkKey, Config{
		Logging:    sharedtest.TestLogging(),
		DataSource: sharedtest.SingleDataSourceFactory{Instance: dataSource},
		Events:     ldcomponents.NoEvents(),
	}, time.Second)

	assert.NotNil(t, client)
	assert.Equal(t, err, ErrInitializationFailed)
}

func makeTestClient() *LDClient {
	return makeTestClientWithConfig(nil)
}

func makeTestClientWithConfig(modConfig func(*Config)) *LDClient {
	config := Config{
		Offline:   false,
		DataStore: ldcomponents.InMemoryDataStore(),
		DataSource: sharedtest.SingleDataSourceFactory{
			Instance: sharedtest.MockDataSource{Initialized: true},
		},
		Events:  sharedtest.SingleEventProcessorFactory{Instance: &sharedtest.CapturingEventProcessor{}},
		Logging: sharedtest.TestLogging(),
	}
	if modConfig != nil {
		modConfig(&config)
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	return client
}
