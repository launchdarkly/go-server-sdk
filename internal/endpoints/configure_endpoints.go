package endpoints

import (
	"strings"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
)

// ServiceType is used internally to denote which endpoint a URI is for.
type ServiceType int

const (
	StreamingService ServiceType = iota //nolint:revive // internal constant
	PollingService   ServiceType = iota //nolint:revive // internal constant
	EventsService    ServiceType = iota //nolint:revive // internal constant
)

func (s ServiceType) String() string {
	switch s {
	case StreamingService:
		return "Streaming"
	case PollingService:
		return "Polling"
	case EventsService:
		return "Events"
	default:
		return "???"
	}
}

func anyCustom(serviceEndpoints interfaces.ServiceEndpoints) bool {
	return serviceEndpoints.Streaming != "" || serviceEndpoints.Polling != "" ||
		serviceEndpoints.Events != ""
}

func getCustom(serviceEndpoints interfaces.ServiceEndpoints, serviceType ServiceType) string {
	switch serviceType {
	case StreamingService:
		return serviceEndpoints.Streaming
	case PollingService:
		return serviceEndpoints.Polling
	case EventsService:
		return serviceEndpoints.Events
	default:
		return ""
	}
}

// IsCustom returns true if the service endpoint has been overridden with a non-default value.
func IsCustom(serviceEndpoints interfaces.ServiceEndpoints, serviceType ServiceType, overrideValue string) bool {
	uri := overrideValue
	if uri == "" {
		uri = getCustom(serviceEndpoints, serviceType)
	}
	return uri != "" && strings.TrimSuffix(uri, "/") != strings.TrimSuffix(DefaultBaseURI(serviceType), "/")
}

// DefaultBaseURI returns the default base URI for the given kind of endpoint.
func DefaultBaseURI(serviceType ServiceType) string {
	switch serviceType {
	case StreamingService:
		return DefaultStreamingBaseURI
	case PollingService:
		return DefaultPollingBaseURI
	case EventsService:
		return DefaultEventsBaseURI
	default:
		return ""
	}
}

// SelectBaseURI is a helper for getting either a custom or a default URI for the given kind of endpoint.
func SelectBaseURI(
	serviceEndpoints interfaces.ServiceEndpoints,
	serviceType ServiceType,
	overrideValue string,
	loggers ldlog.Loggers,
) string {
	configuredBaseURI := overrideValue
	if configuredBaseURI == "" {
		if anyCustom(serviceEndpoints) {
			configuredBaseURI = getCustom(serviceEndpoints, serviceType)
			if configuredBaseURI == "" {
				loggers.Errorf(
					"You have set custom ServiceEndpoints without specifying the %s base URI; connections may not work properly",
					serviceType,
				)
				configuredBaseURI = DefaultBaseURI(serviceType)
			}
		} else {
			configuredBaseURI = DefaultBaseURI(serviceType)
		}
	}
	return strings.TrimRight(configuredBaseURI, "/")
}

// AddPath concatenates a subpath to a URL in a way that will not cause a double slash.
func AddPath(baseURI string, path string) string {
	return strings.TrimSuffix(baseURI, "/") + "/" + strings.TrimPrefix(path, "/")
}
