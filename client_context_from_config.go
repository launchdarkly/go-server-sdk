package ldclient

import (
	"net/http"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"

	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
)

func newClientContextFromConfig(
	sdkKey string,
	config Config,
	diagnosticsManager *ldevents.DiagnosticsManager,
) interfaces.ClientContext {
	headers := make(http.Header)
	headers.Set("Authorization", sdkKey)
	headers.Set("User-Agent", config.UserAgent)
	if config.WrapperName != "" {
		w := config.WrapperName
		if config.WrapperVersion != "" {
			w = w + "/" + config.WrapperVersion
		}
		headers.Add("X-LaunchDarkly-Wrapper", w)
	}
	httpClientFactoryFromConfig := config.HTTPClientFactory
	if httpClientFactoryFromConfig == nil {
		httpClientFactoryFromConfig = NewHTTPClientFactory()
	}
	httpClientFactory := func() *http.Client {
		client := httpClientFactoryFromConfig(config)
		return &client
	}
	return internal.NewClientContextImpl(
		sdkKey,
		config.Loggers,
		headers,
		httpClientFactory,
		config.Offline,
		diagnosticsManager,
	)
}
