package ldclient

import (
	"errors"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"

	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
)

func newClientContextFromConfig(
	sdkKey string,
	config Config,
	diagnosticsManager *ldevents.DiagnosticsManager,
) (interfaces.ClientContext, error) {
	if !stringIsValidHTTPHeaderValue(sdkKey) {
		// We want to fail fast in this case, because if we got as far as trying to make an HTTP request
		// to LaunchDarkly with a malformed key, the Go HTTP client unfortunately would include the
		// actual Authorization header value in its error message, which could end up in logs - and the
		// value might be a real SDK key that just has (for instance) a newline at the end of it, so it
		// would be sensitive information.
		return nil, errors.New("SDK key contains invalid characters")
	}

	basicConfig := interfaces.BasicConfiguration{SDKKey: sdkKey, Offline: config.Offline}

	httpFactory := config.HTTP
	if httpFactory == nil {
		httpFactory = ldcomponents.HTTPConfiguration()
	}
	http, err := httpFactory.CreateHTTPConfiguration(basicConfig)
	if err != nil {
		return nil, err
	}

	loggingFactory := config.Logging
	if loggingFactory == nil {
		loggingFactory = ldcomponents.Logging()
	}
	logging, err := loggingFactory.CreateLoggingConfiguration(basicConfig)
	if err != nil {
		return nil, err
	}

	return internal.NewClientContextImpl(
		sdkKey,
		http,
		logging,
		config.Offline,
		diagnosticsManager,
	), nil
}

func stringIsValidHTTPHeaderValue(s string) bool {
	for _, ch := range s {
		if ch < 32 || ch > 127 {
			return false
		}
	}
	return true
}
