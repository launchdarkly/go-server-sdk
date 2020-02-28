package interfaces

import "gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"

// EventProcessorFactory is a factory that creates some implementation of EventProcessor.
//
// The EventProcessor component is responsible for computing and sending analytics events. Applications
// will normally use one of two implementations: ldcomponents.SendEvents(), which enables events and
// provides builder options for configuring them, or ldcomponents.NoEvents(), which disables events.

// The interface and its standard implementation are defined in the go-sdk-events package (which is in a
// separate repository because it is also used by other LaunchDarkly components) and applications normally
// do not need to interact with it directly, except for testing purposes or if an alternate event storage
// mechanism is needed.
type EventProcessorFactory interface {
	CreateEventProcessor(context ClientContext) (ldevents.EventProcessor, error)
}
