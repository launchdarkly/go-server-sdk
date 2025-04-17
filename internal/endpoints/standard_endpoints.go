package endpoints

const (
	// DefaultStreamingBaseURI is the default base URI of the streaming service.
	DefaultStreamingBaseURI = "https://stream.launchdarkly.com/"

	// DefaultPollingBaseURI is the default base URI of the polling service.
	DefaultPollingBaseURI = "https://sdk.launchdarkly.com/"

	// DefaultEventsBaseURI is the default base URI of the events service.
	DefaultEventsBaseURI = "https://events.launchdarkly.com/"

	// StreamingRequestPath is the URL path for the server-side streaming endpoint.
	StreamingRequestPath = "/all"

	// PollingRequestPath is the URL path for the server-side polling endpoint.
	PollingRequestPath = "/sdk/latest-all"

	// PollingRequestV2Path is the URL path for the server-side polling endpoint v2.
	PollingRequestV2Path = "/sdk/poll"

	// Events service paths are defined in the go-sdk-events package
)
