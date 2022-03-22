package ldcomponents

import "github.com/launchdarkly/go-server-sdk/v6/interfaces"

// RelayProxyEndpoints specifies a single base URI for a Relay Proxy instance, telling the SDK to
// use the Relay Proxy for all services.
//
// When using the LaunchDarkly Relay Proxy (https://docs.launchdarkly.com/home/relay-proxy), the SDK
// only needs to know the single base URI of the Relay Proxy, which will provide all of the proxied
// service endpoints.
//
// Store this value in the ServiceEndpoints field of your SDK configuration. For example:
//
//      relayURI := "http://my-relay-hostname:8080"
//      config := ld.Config{
//          ServiceEndpoints: ldcomponents.RelayProxyEndpoints(relayURI),
//      }
//
// If analytics events are enabled, this will also cause the SDK to forward events through the
// Relay Proxy. If you have not enabled event forwarding in your Relay Proxy configuration and you
// want the SDK to send events directly to LaunchDarkly instead, use RelayProxyEndpointsWithoutEvents.
//
// See Config.ServiceEndpoints for more details.
func RelayProxyEndpoints(relayProxyBaseURI string) interfaces.ServiceEndpoints {
	return interfaces.ServiceEndpoints{
		Streaming: relayProxyBaseURI,
		Polling:   relayProxyBaseURI,
		Events:    relayProxyBaseURI,
	}
}

// RelayProxyEndpointsWithoutEvents specifies a single base URI for a Relay Proxy instance, telling
// the SDK to use the Relay Proxy for all services except analytics events.
//
// When using the LaunchDarkly Relay Proxy (https://docs.launchdarkly.com/home/relay-proxy), the SDK
// only needs to know the single base URI of the Relay Proxy, which will provide all of the proxied
// service endpoints.
//
// Store this value in the ServiceEndpoints field of your SDK configuration. For example:
//
//      relayURI := "http://my-relay-hostname:8080"
//      config := ld.Config{
//          ServiceEndpoints: ldcomponents.RelayProxyEndpointsWithoutEvents(relayURI),
//      }
//
// If you do want events to be forwarded through the Relay Proxy, use RelayProxyEndpoints instead.
//
// See Config.ServiceEndpoints for more details.
func RelayProxyEndpointsWithoutEvents(relayProxyBaseURI string) interfaces.ServiceEndpoints {
	return interfaces.ServiceEndpoints{
		Streaming: relayProxyBaseURI,
		Polling:   relayProxyBaseURI,
	}
}
