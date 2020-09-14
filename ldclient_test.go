package ldclient

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	"github.com/stretchr/testify/assert"
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

func TestMakeCustomClientWithFailedInitialization(t *testing.T) {
	client, err := MakeCustomClient(testSdkKey, Config{
		Logging:    sharedtest.TestLogging(),
		DataSource: sharedtest.DataSourceThatNeverInitializes(),
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
		Offline:    false,
		DataStore:  ldcomponents.InMemoryDataStore(),
		DataSource: sharedtest.DataSourceThatIsAlwaysInitialized(),
		Events:     sharedtest.SingleEventProcessorFactory{Instance: &sharedtest.CapturingEventProcessor{}},
		Logging:    sharedtest.TestLogging(),
	}
	if modConfig != nil {
		modConfig(&config)
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	return client
}
