package ldclient

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"

	"github.com/launchdarkly/go-sdk-common/v3/lduser"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSdkKey = "test-sdk-key"

func TestErrorFromComponentFactoryStopsClientCreation(t *testing.T) {
	fakeError := errors.New("sorry")

	doTest := func(name string, config Config, expectedError error) {
		t.Run(name, func(t *testing.T) {
			client, err := MakeCustomClient(testSdkKey, config, 0)
			assert.Nil(t, client)
			require.Error(t, err)
			assert.Equal(t, expectedError.Error(), err.Error())
		})
	}

	doTest("DataSource", Config{DataSource: mocks.ComponentConfigurerThatReturnsError[subsystems.DataSource]{Err: fakeError}}, fakeError)
	doTest("DataStore", Config{DataStore: mocks.ComponentConfigurerThatReturnsError[subsystems.DataStore]{Err: fakeError}}, fakeError)
	doTest("Events", Config{Events: mocks.ComponentConfigurerThatReturnsError[ldevents.EventProcessor]{Err: fakeError}}, fakeError)
	doTest("HTTP", Config{HTTP: ldcomponents.HTTPConfiguration().CACert([]byte{1})}, errors.New("invalid CA certificate data"))
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
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
		DataSource: mocks.DataSourceThatNeverInitializes(),
		Events:     ldcomponents.NoEvents(),
	}, time.Second)

	assert.NotNil(t, client)
	assert.Equal(t, err, ErrInitializationFailed)
}

func TestInvalidCharacterInSDKKey(t *testing.T) {
	badKey := "my-bad-key\n"

	_, err := MakeClient(badKey, time.Minute)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "my-bad-key") // message shouldn't include the key value

	_, err = MakeCustomClient(badKey, Config{}, time.Minute)
	assert.Error(t, err)
	assert.NotContains(t, err.Error(), "my-bad-key")
}

func makeTestClient() *LDClient {
	return makeTestClientWithConfig(nil)
}

func makeTestClientWithConfig(modConfig func(*Config)) *LDClient {
	config := Config{
		Offline:    false,
		DataStore:  ldcomponents.InMemoryDataStore(),
		DataSource: mocks.DataSourceThatIsAlwaysInitialized(),
		Events:     mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: &mocks.CapturingEventProcessor{}},
		Logging:    ldcomponents.Logging().Loggers(sharedtest.NewTestLoggers()),
	}
	if modConfig != nil {
		modConfig(&config)
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	return client
}
