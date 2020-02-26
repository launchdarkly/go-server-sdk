package ldclient

import (
	"net/http"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldhttp"
)

// DefaultTimeout is the HTTP timeout used if Config.Timeout is not set.
const DefaultTimeout = 3 * time.Second

// Config exposes advanced configuration options for the LaunchDarkly client.
//
// All of these settings are optional, so an empty Config struct is always valid. See the description of each
// field for the default behavior if it is not set.
type Config struct {
	// Sets the implementation of DataSource for receiving feature flag updates.
	//
	// If nil, the default is ldcomponents.StreamingDataSource(); see that method for an explanation of how to
	// further configure streaming behavior. Other options include ldcomponents.PollingDataSource(),
	// ldcomponents.ExternalUpdatesOnly(), the file data source in ldfiledata, or a custom implementation for testing.
	//
	// If Offline is set to true, then DataSource is ignored.
	DataSource interfaces.DataSourceFactory
	// Sets the implementation of DataStore for holding feature flags and related data received from
	// LaunchDarkly.
	//
	// If nil, the default is ldcomponents.InMemoryDataStore(). Other available implementations include the
	// database integrations in the redis, ldconsul, and lddynamodb packages.
	DataStore interfaces.DataStoreFactory
	// Set to true to opt out of sending diagnostic events.
	//
	// Unless DiagnosticOptOut is set to true, the client will send some diagnostics data to the LaunchDarkly
	// servers in order to assist in the development of future SDK improvements. These diagnostics consist of an
	// initial payload containing some details of the SDK in use, the SDK's configuration, and the platform the
	// SDK is being run on, as well as payloads sent periodically with information on irregular occurrences such
	// as dropped events.
	DiagnosticOptOut bool
	// Sets the SDK's behavior regarding analytics events.
	//
	// If nil, the default is ldcomponents.SendEvents(); see that method for an explanation of how to further
	// configure event delivery. You may also turn off event delivery using ldcomponents.NoEvents().
	//
	// If Offline is set to true, then event delivery is always off and Events is ignored.
	Events interfaces.EventProcessorFactory
	// If not nil, this function will be called to create an HTTP client instead of using the default
	// client. You may use this to specify custom HTTP properties such as a proxy URL or CA certificates.
	// The SDK may modify the client properties after that point (for instance, to add caching),
	// but will not replace the underlying Transport, and will not modify any timeout properties you set.
	// See NewHTTPClientFactory().
	//
	//     config := ld.DefaultConfig
	//     config.HTTPClientFactory = ld.NewHTTPClientFactory(ldhttp.ProxyURL(myProxyURL))
	HTTPClientFactory HTTPClientFactory
	// Sets whether the client should log a warning message whenever a flag cannot be evaluated due to an error
	// (e.g. there is no flag with that key, or the user properties are invalid). By default, these messages are
	// not logged, although you can detect such errors programmatically using the VariationDetail methods.
	LogEvaluationErrors bool
	// Sets whether log messages for errors related to a specific user can include the user key. By default, they
	// will not, since the user key might be considered privileged information.
	LogUserKeyInErrors bool
	// Configures the SDK's logging behavior. You may call its SetBaseLogger() method to specify the
	// output destination (the default is standard error), and SetMinLevel() to specify the minimum level
	// of messages to be logged (the default is ldlog.Info).
	Loggers ldlog.Loggers
	// Sets whether this client is offline. An offline client will not make any network connections to LaunchDarkly,
	// and will return default values for all feature flags.
	Offline bool
	// The connection timeout to use when making polling requests to LaunchDarkly.
	//
	// The default is three seconds.
	Timeout time.Duration
	// The User-Agent header to send with HTTP requests. This defaults to a value that identifies the version
	// of the Go SDK for LaunchDarkly usage metrics.
	UserAgent string
	// For use by wrapper libraries to set an identifying name for the wrapper being used.
	//
	// This will be sent in request headers during requests to the LaunchDarkly servers to allow recording
	// metrics on the usage of these wrapper libraries.
	WrapperName string
	// For use by wrapper libraries to set the version to be included alongside a WrapperName.
	//
	// If WrapperName is unset, this field will be ignored.
	WrapperVersion string
}

// HTTPClientFactory is a function that creates a custom HTTP client.
type HTTPClientFactory func(Config) http.Client

func (c Config) newHTTPClient() *http.Client {
	factory := c.HTTPClientFactory
	if factory == nil {
		factory = NewHTTPClientFactory()
	}
	client := factory(c)
	return &client
}

// NewHTTPClientFactory creates an HTTPClientFactory based on the standard SDK configuration as well
// as any custom ldhttp.TransportOption properties you specify.
//
//     config := ld.DefaultConfig
//     config.HTTPClientFactory = ld.NewHTTPClientFactory(ldhttp.CACertFileOption("my-cert.pem"))
func NewHTTPClientFactory(options ...ldhttp.TransportOption) HTTPClientFactory {
	return func(c Config) http.Client {
		client := http.Client{
			Timeout: c.Timeout,
		}
		if c.Timeout <= 0 {
			client.Timeout = DefaultTimeout
		}
		allOpts := []ldhttp.TransportOption{ldhttp.ConnectTimeoutOption(c.Timeout)}
		allOpts = append(allOpts, options...)
		if transport, _, err := ldhttp.NewHTTPTransport(allOpts...); err == nil {
			client.Transport = transport
		}
		return client
	}
}
