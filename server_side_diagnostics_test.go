package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

var testStartWaitMillis = time.Second * 10

func expectedDiagnosticConfigForDefaultConfig() ldvalue.ObjectBuilder {
	return ldvalue.ObjectBuild().
		Set("customBaseURI", ldvalue.Bool(false)).
		Set("customEventsURI", ldvalue.Bool(false)).
		Set("customStreamURI", ldvalue.Bool(false)).
		Set("dataStoreType", ldvalue.String("memory")).
		Set("eventsCapacity", ldvalue.Int(DefaultConfig.Capacity)).
		Set("connectTimeoutMillis", durationToMillis(DefaultConfig.Timeout)).
		Set("socketTimeoutMillis", durationToMillis(DefaultConfig.Timeout)).
		Set("eventsFlushIntervalMillis", durationToMillis(DefaultConfig.FlushInterval)).
		Set("pollingIntervalMillis", durationToMillis(DefaultConfig.PollInterval)).
		Set("startWaitMillis", durationToMillis(testStartWaitMillis)).
		Set("reconnectTimeMillis", ldvalue.Int(3000)).
		Set("streamingDisabled", ldvalue.Bool(false)).
		Set("usingRelayDaemon", ldvalue.Bool(false)).
		Set("allAttributesPrivate", ldvalue.Bool(false)).
		Set("inlineUsersInEvents", ldvalue.Bool(false)).
		Set("userKeysCapacity", ldvalue.Int(DefaultConfig.UserKeysCapacity)).
		Set("userKeysFlushIntervalMillis", durationToMillis(DefaultConfig.UserKeysFlushInterval)).
		Set("usingProxy", ldvalue.Bool(false)).
		Set("diagnosticRecordingIntervalMillis", durationToMillis(DefaultConfig.DiagnosticRecordingInterval))
}

func TestDiagnosticEventCustomConfig(t *testing.T) {
	tests := []struct {
		setConfig   func(*Config)
		setExpected func(ldvalue.ObjectBuilder)
	}{
		{func(c *Config) {}, func(b ldvalue.ObjectBuilder) {}},
		{func(c *Config) { c.BaseUri = "custom" }, func(b ldvalue.ObjectBuilder) { b.Set("customBaseURI", ldvalue.Bool(true)) }},
		{func(c *Config) { c.EventsUri = "custom" }, func(b ldvalue.ObjectBuilder) { b.Set("customEventsURI", ldvalue.Bool(true)) }},
		{func(c *Config) { c.StreamUri = "custom" }, func(b ldvalue.ObjectBuilder) { b.Set("customStreamURI", ldvalue.Bool(true)) }},
		{func(c *Config) {
			f := NewInMemoryDataStoreFactory()
			c.DataStore, _ = f(DefaultConfig)
		},
			func(b ldvalue.ObjectBuilder) {
				b.Set("dataStoreType", ldvalue.String("memory"))
			}},
		{func(c *Config) { c.DataStore = customStoreForDiagnostics{name: "Foo"} },
			func(b ldvalue.ObjectBuilder) {
				b.Set("dataStoreType", ldvalue.String("Foo"))
			}},
		// Can't use our actual persistent store implementations (Redis, etc.) in this test because it'd be
		// a circular package reference. There are tests in each of those packages to verify that they
		// return the expected component type names.
		{func(c *Config) { c.Capacity = 99 }, func(b ldvalue.ObjectBuilder) { b.Set("eventsCapacity", ldvalue.Int(99)) }},
		{func(c *Config) { c.Timeout = time.Second }, func(b ldvalue.ObjectBuilder) {
			b.Set("connectTimeoutMillis", ldvalue.Int(1000))
			b.Set("socketTimeoutMillis", ldvalue.Int(1000))
		}},
		{func(c *Config) { c.FlushInterval = time.Second }, func(b ldvalue.ObjectBuilder) { b.Set("eventsFlushIntervalMillis", ldvalue.Int(1000)) }},
		{func(c *Config) { c.PollInterval = time.Second }, func(b ldvalue.ObjectBuilder) { b.Set("pollingIntervalMillis", ldvalue.Int(1000)) }},
		{func(c *Config) { c.Stream = false }, func(b ldvalue.ObjectBuilder) { b.Set("streamingDisabled", ldvalue.Bool(true)) }},
		{func(c *Config) { c.UseLdd = true }, func(b ldvalue.ObjectBuilder) { b.Set("usingRelayDaemon", ldvalue.Bool(true)) }},
		{func(c *Config) { c.AllAttributesPrivate = true }, func(b ldvalue.ObjectBuilder) { b.Set("allAttributesPrivate", ldvalue.Bool(true)) }},
		{func(c *Config) { c.InlineUsersInEvents = true }, func(b ldvalue.ObjectBuilder) { b.Set("inlineUsersInEvents", ldvalue.Bool(true)) }},
		{func(c *Config) { c.UserKeysCapacity = 2 }, func(b ldvalue.ObjectBuilder) { b.Set("userKeysCapacity", ldvalue.Int(2)) }},
		{func(c *Config) { c.UserKeysFlushInterval = time.Second }, func(b ldvalue.ObjectBuilder) { b.Set("userKeysFlushIntervalMillis", ldvalue.Int(1000)) }},
		{func(c *Config) { c.DiagnosticRecordingInterval = time.Second }, func(b ldvalue.ObjectBuilder) { b.Set("diagnosticRecordingIntervalMillis", ldvalue.Int(1000)) }},
	}
	for _, test := range tests {
		config := DefaultConfig
		config.DataStore, _ = NewInMemoryDataStoreFactory()(DefaultConfig)
		test.setConfig(&config)
		expected := expectedDiagnosticConfigForDefaultConfig()
		test.setExpected(expected)

		actual := makeDiagnosticConfigData(config, testStartWaitMillis)
		assert.Equal(t, expected.Build(), actual)
	}
}

type customStoreForDiagnostics struct {
	name string
}

func (c customStoreForDiagnostics) GetDiagnosticsComponentTypeName() string {
	return c.name
}

func (c customStoreForDiagnostics) Get(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	return nil, nil
}

func (c customStoreForDiagnostics) All(kind interfaces.VersionedDataKind) (map[string]interfaces.VersionedData, error) {
	return nil, nil
}

func (c customStoreForDiagnostics) Init(data map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData) error {
	return nil
}

func (c customStoreForDiagnostics) Delete(kind interfaces.VersionedDataKind, key string, version int) error {
	return nil
}

func (c customStoreForDiagnostics) Upsert(kind interfaces.VersionedDataKind, item interfaces.VersionedData) error {
	return nil
}

func (c customStoreForDiagnostics) Initialized() bool {
	return false
}
