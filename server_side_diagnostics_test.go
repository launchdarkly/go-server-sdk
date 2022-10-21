package ldclient

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"

	"github.com/stretchr/testify/assert"
)

var testStartWaitMillis = time.Second * 10

func expectedDiagnosticConfigForDefaultConfig() *ldvalue.ObjectBuilder {
	return ldvalue.ObjectBuild().
		Set("customEventsURI", ldvalue.Bool(false)).
		Set("dataStoreType", ldvalue.String("memory")).
		Set("eventsCapacity", ldvalue.Int(ldcomponents.DefaultEventsCapacity)).
		Set("connectTimeoutMillis", durationToMillis(ldcomponents.DefaultConnectTimeout)).
		Set("socketTimeoutMillis", durationToMillis(ldcomponents.DefaultConnectTimeout)).
		Set("eventsFlushIntervalMillis", durationToMillis(ldcomponents.DefaultFlushInterval)).
		Set("startWaitMillis", durationToMillis(testStartWaitMillis)).
		Set("usingRelayDaemon", ldvalue.Bool(false)).
		Set("allAttributesPrivate", ldvalue.Bool(false)).
		Set("userKeysCapacity", ldvalue.Int(ldcomponents.DefaultContextKeysCapacity)).
		Set("userKeysFlushIntervalMillis", durationToMillis(ldcomponents.DefaultContextKeysFlushInterval)).
		Set("usingProxy", ldvalue.Bool(false)).
		Set("diagnosticRecordingIntervalMillis", durationToMillis(ldcomponents.DefaultDiagnosticRecordingInterval))
}

func TestDiagnosticEventCustomConfig(t *testing.T) {
	timeMillis := func(t time.Duration) ldvalue.Value { return ldvalue.Int(int(t / time.Millisecond)) }
	doTestWithoutStreamingDefaults := func(setConfig func(*Config), setExpected func(*ldvalue.ObjectBuilder)) {
		config := Config{}
		setConfig(&config)
		expected := expectedDiagnosticConfigForDefaultConfig()
		setExpected(expected)
		context, _ := newClientContextFromConfig(testSdkKey, config)
		actual := makeDiagnosticConfigData(context, config, testStartWaitMillis)
		assert.JSONEq(t, expected.Build().JSONString(), actual.JSONString())
	}
	doTest := func(setConfig func(*Config), setExpected func(*ldvalue.ObjectBuilder)) {
		doTestWithoutStreamingDefaults(setConfig, func(b *ldvalue.ObjectBuilder) {
			b.SetBool("customStreamURI", false).
				Set("reconnectTimeMillis", timeMillis(ldcomponents.DefaultInitialReconnectDelay)).
				SetBool("streamingDisabled", false)
			setExpected(b)
		})
	}

	doTest(func(c *Config) {}, func(b *ldvalue.ObjectBuilder) {})

	// data store configuration
	doTest(func(c *Config) { c.DataStore = ldcomponents.InMemoryDataStore() }, func(b *ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) { c.DataStore = customStoreFactoryForDiagnostics{name: "Foo"} },
		func(b *ldvalue.ObjectBuilder) { b.SetString("dataStoreType", "Foo") })
	doTest(func(c *Config) { c.DataStore = customStoreFactoryWithoutDiagnosticDescription{} },
		func(b *ldvalue.ObjectBuilder) { b.SetString("dataStoreType", "custom") })

	// data source configuration
	doTest(func(c *Config) { c.DataSource = ldcomponents.StreamingDataSource() }, func(b *ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) {
		c.ServiceEndpoints = interfaces.ServiceEndpoints{Streaming: "custom"}
	}, func(b *ldvalue.ObjectBuilder) {
		b.SetBool("customStreamURI", true)
	})
	doTest(func(c *Config) { c.DataSource = ldcomponents.StreamingDataSource().InitialReconnectDelay(time.Minute) },
		func(b *ldvalue.ObjectBuilder) { b.Set("reconnectTimeMillis", ldvalue.Int(60000)) })
	doTestWithoutStreamingDefaults(func(c *Config) { c.DataSource = ldcomponents.PollingDataSource() }, func(b *ldvalue.ObjectBuilder) {
		b.SetBool("streamingDisabled", true)
		b.SetBool("customBaseURI", false)
		b.Set("pollingIntervalMillis", timeMillis(ldcomponents.DefaultPollInterval))
	})
	doTestWithoutStreamingDefaults(func(c *Config) {
		c.DataSource = ldcomponents.PollingDataSource().PollInterval(time.Minute * 99)
	}, func(b *ldvalue.ObjectBuilder) {
		b.SetBool("streamingDisabled", true)
		b.SetBool("customBaseURI", false)
		b.Set("pollingIntervalMillis", timeMillis(time.Minute*99))
	})
	doTestWithoutStreamingDefaults(func(c *Config) {
		c.DataSource = ldcomponents.PollingDataSource()
		c.ServiceEndpoints = interfaces.ServiceEndpoints{Polling: "custom"}
	}, func(b *ldvalue.ObjectBuilder) {
		b.SetBool("streamingDisabled", true)
		b.SetBool("customBaseURI", true)
		b.Set("pollingIntervalMillis", timeMillis(ldcomponents.DefaultPollInterval))
	})
	doTestWithoutStreamingDefaults(func(c *Config) { c.DataSource = ldcomponents.ExternalUpdatesOnly() },
		func(b *ldvalue.ObjectBuilder) { b.SetBool("usingRelayDaemon", true) })

	// events configuration
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents() }, func(b *ldvalue.ObjectBuilder) {})
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().AllAttributesPrivate(true) },
		func(b *ldvalue.ObjectBuilder) { b.SetBool("allAttributesPrivate", true) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().DiagnosticRecordingInterval(time.Second * 99) },
		func(b *ldvalue.ObjectBuilder) { b.SetInt("diagnosticRecordingIntervalMillis", 99000) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().Capacity(99) },
		func(b *ldvalue.ObjectBuilder) { b.SetInt("eventsCapacity", 99) })
	doTest(func(c *Config) { c.ServiceEndpoints = interfaces.ServiceEndpoints{Events: "custom"} },
		func(b *ldvalue.ObjectBuilder) { b.SetBool("customEventsURI", true) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().FlushInterval(time.Second) },
		func(b *ldvalue.ObjectBuilder) { b.SetInt("eventsFlushIntervalMillis", 1000) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().ContextKeysCapacity(2) },
		func(b *ldvalue.ObjectBuilder) { b.SetInt("userKeysCapacity", 2) })
	doTest(func(c *Config) { c.Events = ldcomponents.SendEvents().ContextKeysFlushInterval(time.Second) },
		func(b *ldvalue.ObjectBuilder) { b.Set("userKeysFlushIntervalMillis", ldvalue.Int(1000)) })

	// network properties
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().ConnectTimeout(time.Second)
		},
		func(b *ldvalue.ObjectBuilder) {
			b.SetInt("connectTimeoutMillis", 1000)
			b.SetInt("socketTimeoutMillis", 1000)
		})
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().ProxyURL("http://proxyhost")
		},
		func(b *ldvalue.ObjectBuilder) {
			b.SetBool("usingProxy", true)
		})
	doTest(
		func(c *Config) {
			c.HTTP = ldcomponents.HTTPConfiguration().
				HTTPClientFactory(func() *http.Client { return http.DefaultClient })
		},
		func(b *ldvalue.ObjectBuilder) {})
	func() {
		os.Setenv("HTTP_PROXY", "http://proxyhost")
		defer os.Setenv("HTTP_PROXY", "")
		doTest(
			func(c *Config) {},
			func(b *ldvalue.ObjectBuilder) {
				b.SetBool("usingProxy", true)
			})
	}()
}

type customStoreFactoryForDiagnostics struct {
	name string
}

func (c customStoreFactoryForDiagnostics) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	return ldvalue.String(c.name)
}

func (c customStoreFactoryForDiagnostics) Build(context subsystems.ClientContext) (subsystems.DataStore, error) {
	return nil, errors.New("not implemented")
}

type customStoreFactoryWithoutDiagnosticDescription struct{}

func (c customStoreFactoryWithoutDiagnosticDescription) Build(context subsystems.ClientContext) (subsystems.DataStore, error) {
	return nil, errors.New("not implemented")
}
