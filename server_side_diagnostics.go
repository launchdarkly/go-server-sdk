package ldclient

import (
	"os"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

func createDiagnosticsManager(sdkKey string, config Config, waitFor time.Duration) *ldevents.DiagnosticsManager {
	id := ldevents.NewDiagnosticID(sdkKey)
	return ldevents.NewDiagnosticsManager(id, makeDiagnosticConfigData(config, waitFor), makeDiagnosticSDKData(), time.Now(), nil)
}

func makeDiagnosticConfigData(config Config, waitFor time.Duration) ldvalue.Value {
	// Notes on config data
	// - reconnectTimeMillis: hard-coded in eventsource because we're not overriding StreamOptionInitialRetry.
	// - usingProxy: there are many ways to implement an HTTP proxy in Go, but the only one we're capable of
	//   detecting is the HTTP_PROXY environment variable; programmatic approaches involve using a custom
	//   transport, which we have no way of distinguishing from other kinds of custom transports (for the
	//   same reason, we cannot detect if proxy authentication is being used).
	return ldvalue.ObjectBuild().
		Set("customBaseURI", ldvalue.Bool(config.BaseUri != DefaultConfig.BaseUri)).
		Set("customEventsURI", ldvalue.Bool(config.EventsUri != DefaultConfig.EventsUri)).
		Set("customStreamURI", ldvalue.Bool(config.StreamUri != DefaultConfig.StreamUri)).
		Set("dataStoreType", ldvalue.String(getComponentTypeName(config.DataStore).OrElse("memory"))).
		Set("eventsCapacity", ldvalue.Int(config.Capacity)).
		Set("connectTimeoutMillis", durationToMillis(config.Timeout)).
		Set("socketTimeoutMillis", durationToMillis(config.Timeout)).
		Set("eventsFlushIntervalMillis", durationToMillis(config.FlushInterval)).
		Set("pollingIntervalMillis", durationToMillis(config.PollInterval)).
		Set("startWaitMillis", durationToMillis(waitFor)).
		Set("reconnectTimeMillis", ldvalue.Int(3000)).
		Set("streamingDisabled", ldvalue.Bool(!config.Stream)).
		Set("usingRelayDaemon", ldvalue.Bool(config.UseLdd)).
		Set("allAttributesPrivate", ldvalue.Bool(config.AllAttributesPrivate)).
		Set("inlineUsersInEvents", ldvalue.Bool(config.InlineUsersInEvents)).
		Set("userKeysCapacity", ldvalue.Int(config.UserKeysCapacity)).
		Set("userKeysFlushIntervalMillis", durationToMillis(config.UserKeysFlushInterval)).
		Set("usingProxy", ldvalue.Bool(os.Getenv("HTTP_PROXY") != "")).
		Set("diagnosticRecordingIntervalMillis", durationToMillis(config.DiagnosticRecordingInterval)).
		Build()
}

func makeDiagnosticSDKData() ldvalue.Value {
	return ldvalue.ObjectBuild().
		Set("name", ldvalue.String("go-server-sdk")).
		Set("version", ldvalue.String(Version)).
		Build()
}

func durationToMillis(d time.Duration) ldvalue.Value {
	return ldvalue.Float64(float64(uint64(d / time.Millisecond)))
}

// Optional interface that can be implemented by components whose types can't be easily
// determined by looking at the config object.
type diagnosticsComponentDescriptor interface {
	GetDiagnosticsComponentTypeName() string
}

func getComponentTypeName(component interface{}) ldvalue.OptionalString {
	if component != nil {
		if dcd, ok := component.(diagnosticsComponentDescriptor); ok {
			return ldvalue.NewOptionalString(dcd.GetDiagnosticsComponentTypeName())
		}
		return ldvalue.NewOptionalString("custom")
	}
	return ldvalue.OptionalString{}
}
