package ldcomponents

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/ldhttp"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// DefaultConnectTimeout is the HTTP connection timeout that is used if [HTTPConfigurationBuilder.ConnectTimeout]
// is not set.
const DefaultConnectTimeout = 3 * time.Second

// HTTPConfigurationBuilder contains methods for configuring the SDK's networking behavior.
//
// If you want to set non-default values for any of these properties, create a builder with
// ldcomponents.[HTTPConfiguration](), change its properties with the HTTPConfigurationBuilder methods,
// and store it in the HTTP field of [github.com/launchdarkly/go-server-sdk/v7.Config]:
//
//	    config := ld.Config{
//	        HTTP: ldcomponents.HTTPConfiguration().
//	            ConnectTimeout(3 * time.Second).
//			       ProxyURL(proxyUrl),
//	    }
type HTTPConfigurationBuilder struct {
	inited            bool
	connectTimeout    time.Duration
	httpClientFactory func() *http.Client
	httpOptions       []ldhttp.TransportOption
	proxyURL          string
	userAgent         string
	wrapperIdentifier string
	customHeaders     map[string]string
}

// HTTPConfiguration returns a configuration builder for the SDK's HTTP configuration.
//
//	    config := ld.Config{
//	        HTTP: ldcomponents.HTTPConfiguration().
//	            ConnectTimeout(3 * time.Second).
//			       ProxyURL(proxyUrl),
//	    }
func HTTPConfiguration() *HTTPConfigurationBuilder {
	return &HTTPConfigurationBuilder{}
}

func (b *HTTPConfigurationBuilder) checkValid() bool {
	if b == nil {
		internal.LogErrorNilPointerMethod("HTTPConfigurationBuilder")
		return false
	}
	if !b.inited {
		b.connectTimeout = DefaultConnectTimeout
		b.customHeaders = make(map[string]string)
		b.inited = true
	}
	return true
}

// CACert specifies a CA certificate to be added to the trusted root CA list for HTTPS requests.
//
// If the certificate is not valid, the LDClient constructor will return an error when you try to create
// the client.
func (b *HTTPConfigurationBuilder) CACert(certData []byte) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.httpOptions = append(b.httpOptions, ldhttp.CACertOption(certData))
	}
	return b
}

// CACertFile specifies a CA certificate to be added to the trusted root CA list for HTTPS requests,
// reading the certificate data from a file in PEM format.
//
// If the certificate is not valid or the file does not exist, the LDClient constructor will return an
// error when you try to create the client.
func (b *HTTPConfigurationBuilder) CACertFile(filePath string) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.httpOptions = append(b.httpOptions, ldhttp.CACertFileOption(filePath))
	}
	return b
}

// ConnectTimeout sets the connection timeout.
//
// This is the maximum amount of time to wait for each individual connection attempt to a remote service
// before determining that that attempt has failed. It is not the same as the timeout for initializing the
// SDK client (the waitFor parameter to MakeClient); that is the total length of time that MakeClient
// will wait regardless of how many connection attempts are required.
//
//	config := ld.Config{
//	    HTTP: ldcomponents.ConnectTimeout(),
//	}
func (b *HTTPConfigurationBuilder) ConnectTimeout(connectTimeout time.Duration) *HTTPConfigurationBuilder {
	if b.checkValid() {
		if connectTimeout <= 0 {
			b.connectTimeout = DefaultConnectTimeout
		} else {
			b.connectTimeout = connectTimeout
		}
	}
	return b
}

// HTTPClientFactory specifies a function for creating each HTTP client instance that is used by the SDK.
//
// If you use this option, it overrides any other settings that you may have specified with
// [HTTPConfigurationBuilder.ConnectTimeout] or [HTTPConfigurationBuilder.ProxyURL]; you are responsible
// for setting up any desired custom configuration on the HTTP client. The SDK  may modify the client
// properties after the client is created (for instance, to add caching), but will not replace the
// underlying [http.Transport], and will not modify any timeout properties you set.
func (b *HTTPConfigurationBuilder) HTTPClientFactory(httpClientFactory func() *http.Client) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.httpClientFactory = httpClientFactory
	}
	return b
}

// ProxyURL specifies a proxy URL to be used for all requests. This overrides any setting of the
// HTTP_PROXY, HTTPS_PROXY, or NO_PROXY environment variables.
//
// If the string is not a valid URL, the LDClient constructor will return an error when you try to create
// the client.
//
// To pass basic proxy credentials, use the format 'scheme://username:password@host:port'.
func (b *HTTPConfigurationBuilder) ProxyURL(proxyURL string) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.proxyURL = proxyURL
	}
	return b
}

// Header specifies a custom HTTP header that should be added to all requests. Repeated calls to Header with
// the same key will overwrite previous entries.
//
// This may be helpful if you are using a gateway or proxy server that requires a specific header in
// requests.
//
// Overwriting the User-Agent or Authorization headers is not recommended, as it can interfere with communication
// to LaunchDarkly. To set a custom User Agent, see UserAgent.
func (b *HTTPConfigurationBuilder) Header(key string, value string) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.customHeaders[key] = value
	}
	return b
}

// UserAgent specifies an additional User-Agent header value to send with HTTP requests.
func (b *HTTPConfigurationBuilder) UserAgent(userAgent string) *HTTPConfigurationBuilder {
	if b.checkValid() {
		b.userAgent = userAgent
	}
	return b
}

// Wrapper allows wrapper libraries to set an identifying name for the wrapper being used.
//
// This will be sent in request headers during requests to the LaunchDarkly servers to allow recording
// metrics on the usage of these wrapper libraries.
func (b *HTTPConfigurationBuilder) Wrapper(wrapperName, wrapperVersion string) *HTTPConfigurationBuilder {
	if b.checkValid() {
		if wrapperName == "" || wrapperVersion == "" {
			b.wrapperIdentifier = wrapperName
		} else {
			b.wrapperIdentifier = fmt.Sprintf("%s/%s", wrapperName, wrapperVersion)
		}
	}
	return b
}

// DescribeConfiguration is internally by the SDK to inspect the configuration.
func (b *HTTPConfigurationBuilder) DescribeConfiguration(context subsystems.ClientContext) ldvalue.Value {
	if !b.checkValid() {
		defaults := HTTPConfigurationBuilder{}
		return defaults.DescribeConfiguration(context)
	}
	builder := ldvalue.ObjectBuild()

	builder.Set("connectTimeoutMillis", durationToMillisValue(b.connectTimeout))
	builder.Set("socketTimeoutMillis", durationToMillisValue(b.connectTimeout))

	builder.SetBool("usingProxy", b.isProxyEnabled())

	return builder.Build()
}

func (b *HTTPConfigurationBuilder) isProxyEnabled() bool {
	// There are several ways to implement an HTTP proxy in Go, not all of which we can detect from
	// here. We'll just report this as true if we reasonably suspect there is a proxy; the purpose
	// of this is just for general usage statistics.
	if os.Getenv("HTTP_PROXY") != "" {
		return true
	}
	if b.httpClientFactory != nil {
		return false // for a custom client configuration, we have no way to know how it works
	}
	if b.proxyURL != "" {
		return true
	}
	return false
}

// Build is called internally by the SDK.
func (b *HTTPConfigurationBuilder) Build(
	clientContext subsystems.ClientContext,
) (subsystems.HTTPConfiguration, error) {
	if !b.checkValid() {
		defaults := HTTPConfigurationBuilder{}
		return defaults.Build(clientContext)
	}

	headers := make(http.Header)
	headers.Set("Authorization", clientContext.GetSDKKey())
	userAgent := "GoClient/" + internal.SDKVersion
	if b.userAgent != "" {
		userAgent = userAgent + " " + b.userAgent
	}
	headers.Set("User-Agent", userAgent)
	if b.wrapperIdentifier != "" {
		headers.Add("X-LaunchDarkly-Wrapper", b.wrapperIdentifier)
	}
	if tagsHeaderValue := buildTagsHeaderValue(clientContext); tagsHeaderValue != "" {
		headers.Add("X-LaunchDarkly-Tags", tagsHeaderValue)
	}

	// For consistency with other SDKs, custom headers are allowed to overwrite headers such as
	// User-Agent and Authorization.
	for key, value := range b.customHeaders {
		headers.Set(key, value)
	}

	transportOpts := b.httpOptions

	if b.proxyURL != "" {
		u, err := url.Parse(b.proxyURL)
		if err != nil {
			return subsystems.HTTPConfiguration{}, err
		}
		transportOpts = append(transportOpts, ldhttp.ProxyOption(*u))
	}

	clientFactory := b.httpClientFactory
	if clientFactory == nil {
		connectTimeout := b.connectTimeout
		if connectTimeout <= 0 {
			connectTimeout = DefaultConnectTimeout
		}
		transportOpts = append(transportOpts, ldhttp.ConnectTimeoutOption(connectTimeout))
		transport, _, err := ldhttp.NewHTTPTransport(transportOpts...)
		if err != nil {
			return subsystems.HTTPConfiguration{}, err
		}
		clientFactory = func() *http.Client {
			return &http.Client{
				Timeout:   b.connectTimeout,
				Transport: transport,
			}
		}
	}

	return subsystems.HTTPConfiguration{
		DefaultHeaders:   headers,
		CreateHTTPClient: clientFactory,
	}, nil
}

func buildTagsHeaderValue(clientContext subsystems.ClientContext) string {
	var parts []string
	if value := clientContext.GetApplicationInfo().ApplicationID; value != "" {
		parts = append(parts, fmt.Sprintf("application-id/%s", value))
	}
	if value := clientContext.GetApplicationInfo().ApplicationVersion; value != "" {
		parts = append(parts, fmt.Sprintf("application-version/%s", value))
	}
	return strings.Join(parts, " ")
}
