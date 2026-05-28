package ldclient

import (
	"errors"
	"regexp"

	"github.com/google/uuid"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
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

	// Per SCMP-server-connection-minutes-polling, every outbound request must carry a
	// per-instance UUID v4 in X-LaunchDarkly-Instance-Id. Generate it once here, at the point
	// the client context is materialized, so it is stable for the lifetime of this LDClient and
	// every subsystem built from this context (HTTP layer, data source, event sender) sees the
	// same value.
	basicConfig := subsystems.BasicClientContext{
		SDKKey:           sdkKey,
		InstanceID:       uuid.New().String(),
		Offline:          config.Offline,
		ServiceEndpoints: config.ServiceEndpoints,
	}

	loggingFactory := config.Logging
	if loggingFactory == nil {
		loggingFactory = ldcomponents.Logging()
	}
	logging, err := loggingFactory.Build(basicConfig)
	if err != nil {
		return nil, err
	}
	basicConfig.Logging = logging

	basicConfig.ApplicationInfo.ApplicationID = validateTagValue(config.ApplicationInfo.ApplicationID,
		"ApplicationID", logging.Loggers)
	basicConfig.ApplicationInfo.ApplicationVersion = validateTagValue(config.ApplicationInfo.ApplicationVersion,
		"ApplicationVersion", logging.Loggers)

	httpFactory := config.HTTP
	if httpFactory == nil {
		httpFactory = ldcomponents.HTTPConfiguration()
	}
	http, err := httpFactory.Build(basicConfig)
	if err != nil {
		return nil, err
	}
	basicConfig.HTTP = http

	return &internal.ClientContextImpl{BasicClientContext: basicConfig}, nil
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
	if value == "" {
		return ""
	}
	if len(value) > 64 {
		loggers.Warnf("Value of Config.ApplicationInfo.%s was longer than 64 characters and was discarded", name)
		return ""
	}
	if !validTagKeyOrValueRegex.MatchString(value) {
		loggers.Warnf("Value of Config.ApplicationInfo.%s contained invalid characters and was discarded", name)
		return ""
	}
	return value
}
