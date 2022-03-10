package ldclient

import (
	"errors"
	"regexp"

	"gopkg.in/launchdarkly/go-sdk-common.v3/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v6/ldcomponents"
)

var validTagKeyOrValueRegex = regexp.MustCompile(`(?s)^[\w.-]*$`)

func newClientContextFromConfig(
	sdkKey string,
	config Config,
) (*internal.ClientContextImpl, error) {
	if !stringIsValidHTTPHeaderValue(sdkKey) {
		// We want to fail fast in this case, because if we got as far as trying to make an HTTP request
		// to LaunchDarkly with a malformed key, the Go HTTP client unfortunately would include the
		// actual Authorization header value in its error message, which could end up in logs - and the
		// value might be a real SDK key that just has (for instance) a newline at the end of it, so it
		// would be sensitive information.
		return nil, errors.New("SDK key contains invalid characters")
	}

	basicConfig := interfaces.BasicConfiguration{
		SDKKey:           sdkKey,
		Offline:          config.Offline,
		ServiceEndpoints: config.ServiceEndpoints,
	}

	loggingFactory := config.Logging
	if loggingFactory == nil {
		loggingFactory = ldcomponents.Logging()
	}
	logging, err := loggingFactory.CreateLoggingConfiguration(basicConfig)
	if err != nil {
		return nil, err
	}

	basicConfig.ApplicationInfo.ApplicationID = validateTagValue(config.ApplicationInfo.ApplicationID,
		"ApplicationID", logging.GetLoggers())
	basicConfig.ApplicationInfo.ApplicationVersion = validateTagValue(config.ApplicationInfo.ApplicationVersion,
		"ApplicationVersion", logging.GetLoggers())

	httpFactory := config.HTTP
	if httpFactory == nil {
		httpFactory = ldcomponents.HTTPConfiguration()
	}
	http, err := httpFactory.CreateHTTPConfiguration(basicConfig)
	if err != nil {
		return nil, err
	}

	return internal.NewClientContextImpl(
		basicConfig,
		http,
		logging,
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

func validateTagValue(value, name string, loggers ldlog.Loggers) string {
	if value != "" && !validTagKeyOrValueRegex.MatchString(value) {
		loggers.Warnf("Value of Config.%s contained invalid characters and was discarded", name)
		return ""
	}
	return value
}
