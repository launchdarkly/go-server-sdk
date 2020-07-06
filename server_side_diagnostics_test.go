package ldclient

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

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
		Set("eventsCapacity", ldvalue.Int(ldcomponents.DefaultEventsCapacity)).
		Set("connectTimeoutMillis", durationToMillis(ldcomponents.DefaultConnectTimeout)).
		Set("socketTimeoutMillis", durationToMillis(ldcomponents.DefaultConnectTimeout)).
		Set("eventsFlushIntervalMillis", durationToMillis(ldcomponents.DefaultFlushInterval)).
		Set("startWaitMillis", durationToMillis(testStartWaitMillis)).
		Set("streamingDisabled", ldvalue.Bool(false)).
		Set("usingRelayDaemon", ldvalue.Bool(false)).
		Set("allAttributesPrivate", ldvalue.Bool(false)).
		Set("inlineUsersInEvents", ldvalue.Bool(false)).
		Set("userKeysCapacity", ldvalue.Int(ldcomponents.DefaultUserKeysCapacity)).
		Set("userKeysFlushIntervalMillis", durationToMillis(ldcomponents.DefaultUserKeysFlushInterval)).
		Set("usingProxy", ldvalue.Bool(false)).
		Set("diagnosticRecordingIntervalMillis", durationToMillis(ldcomponents.DefaultDiagnosticRecordingInterval))
}

func TestDiagnosticEventCustomConfig(t *testing.T) {
	timeMillis := func(t time.Duration) ldvalue.Value { return ldvalue.Int(int(t / time.Millisecond)) }
	doTestWithoutStreamingDefaults := func(setConfig func(*Config), setExpected func(ldvalue.ObjectBuilder)) {
		config := Config{}
		setConfig(&config)
		expected := expectedDiagnosticConfigForDefaultConfig()
		setExpected(expected)
		actual := makeDiagnosticConfigData(config, testStartWaitMillis)
		assert.JSONEq(t, expected.Build().JSONString(), actual.JSONString())
	}
	doTest := func(setConfig func(*Config), setExpected func(ldvalue.ObjectBuilder)) {
		doTestWithoutStreamingDefaults(setConfig, func(b ldvalue.ObjectBuilder) {
			b.Set("reconnectTimeMillis", timeMillis(ldcomponents.DefaultInitialReconnectDelay))
			setExpected(b)
		})
	}

	doTest(func(c *Config) {}, func(b ldvalue.ObjectBuilder) {})

	// data store configuration
	doTest(func(c *Config) { c.DataStore = ldcomponents.InMemoryDataStore() }, func(b ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) { c.DataStore = customStoreFactoryForDiagnostics{name: "Foo"} },
		func(b ldvalue.ObjectBuilder) { b.Set("dataStoreType", ldvalue.String("Foo")) })
	doTest(func(c *Config) { c.DataStore = customStoreFactoryWithoutDiagnosticDescription{} },
		func(b ldvalue.ObjectBuilder) { b.Set("dataStoreType", ldvalue.String("custom")) })

	// data source configuration
	doTest(func(c *Config) { c.DataSource = ldcomponents.StreamingDataSource() }, func(b ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) {
		c.DataSource = ldcomponents.StreamingDataSource().BaseURI("custom")
	}, func(b ldvalue.ObjectBuilder) {
		b.Set("customStreamURI", ldvalue.Bool(true))
	})
	doTest(func(c *Config) { c.DataSource = ldcomponents.StreamingDataSource().InitialReconnectDelay(time.Minute) },
		func(b ldvalue.ObjectBuilder) { b.Set("reconnectTimeMillis", ldvalue.Int(60000)) })
	doTestWithoutStreamingDefaults(func(c *Config) { c.DataSource = ldcomponents.PollingDataSource() }, func(b ldvalue.ObjectBuilder) {
		b.Set("streamingDisabled", ldvalue.Bool(true))
		b.Set("pollingIntervalMillis", timeMillis(ldcomponents.DefaultPollInterval))
	})
	doTestWithoutStreamingDefaults(func(c *Config) {
		c.DataSource = ldcomponents.PollingDataSource().BaseURI("custom").PollInterval(time.Minute * 99)
	}, func(b ldvalue.ObjectBuilder) {
		b.Set("streamingDisabled", ldvalue.Bool(true))
		b.Set("customBaseURI", ldvalue.Bool(true))
		b.Set("pollingIntervalMillis", timeMillis(time.Minute*99))
	})
	doTestWithoutStreamingDefaults(func(c *Config) { c.DataSource = ldcomponents.ExternalUpdatesOnly() },
		func(b ldvalue.ObjectBuilder) { b.Set("usingRelayDaemon", ldvalue.Bool(true)) })

	// events configuration
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents() }, func(b ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().AllAttributesPrivate(true) },
		func(b ldvalue.ObjectBuilder) { b.Set("allAttributesPrivate", ldvalue.Bool(true)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().DiagnosticRecordingInterval(time.Second * 99) },
		func(b ldvalue.ObjectBuilder) { b.Set("diagnosticRecordingIntervalMillis", ldvalue.Int(99000)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().Capacity(99) },
		func(b ldvalue.ObjectBuilder) { b.Set("eventsCapacity", ldvalue.Int(99)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().BaseURI("custom") },
		func(b ldvalue.ObjectBuilder) { b.Set("customEventsURI", ldvalue.Bool(true)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().FlushInterval(time.Second) },
		func(b ldvalue.ObjectBuilder) { b.Set("eventsFlushIntervalMillis", ldvalue.Int(1000)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().InlineUsersInEvents(true) },
		func(b ldvalue.ObjectBuilder) { b.Set("inlineUsersInEvents", ldvalue.Bool(true)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().UserKeysCapacity(2) },
		func(b ldvalue.ObjectBuilder) { b.Set("userKeysCapacity", ldvalue.Int(2)) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().UserKeysFlushInterval(time.Second) },
		func(b ldvalue.ObjectBuilder) { b.Set("userKeysFlushIntervalMillis", ldvalue.Int(1000)) })

	// network properties
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().ConnectTimeout(time.Second)
		},
		func(b ldvalue.ObjectBuilder) {
			b.Set("connectTimeoutMillis", ldvalue.Int(1000))
			b.Set("socketTimeoutMillis", ldvalue.Int(1000))
		})
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().ProxyURL("http://proxyhost")
		},
		func(b ldvalue.ObjectBuilder) {
			b.Set("usingProxy", ldvalue.Bool(true))
		})
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().
				HTTPClientFactory(func() *http.Client { return http.DefaultClient })
		},
		func(b ldvalue.ObjectBuilder) {})
	func() {
		os.Setenv("HTTP_PROXY", "http://proxyhost")
		defer os.Setenv("HTTP_PROXY", "")
		doTest(
			func(c *Config) {},
			func(b ldvalue.ObjectBuilder) {
				b.Set("usingProxy", ldvalue.Bool(true))
			})
	}()
}

type customStoreFactoryForDiagnostics struct {
	name string
}

func (c customStoreFactoryForDiagnostics) DescribeConfiguration() ldvalue.Value {
	return ldvalue.String(c.name)
}

func (c customStoreFactoryForDiagnostics) CreateDataStore(
	context interfaces.ClientContext,
	dataStoreUpdates interfaces.DataStoreUpdates,
) (interfaces.DataStore, error) {
	return nil, errors.New("not implemented")
}

type customStoreFactoryWithoutDiagnosticDescription struct{}

func (c customStoreFactoryWithoutDiagnosticDescription) CreateDataStore(
	context interfaces.ClientContext,
	dataStoreUpdates interfaces.DataStoreUpdates,
) (interfaces.DataStore, error) {
	return nil, errors.New("not implemented")
}
