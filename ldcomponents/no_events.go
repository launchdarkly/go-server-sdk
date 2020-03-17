package ldcomponents

import (
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type nullEventProcessorFactory struct{}

// NoEvents returns a configuration object that disables analytics events.
//
// Storing this in Config.Events causes the SDK to discard all analytics events and not send them to
// LaunchDarkly, regardless of any other configuration.
//
//     config := ld.Config{
//         Events: ldcomponents.NoEvents(),
//     }
func NoEvents() interfaces.EventProcessorFactory {
	return nullEventProcessorFactory{}
}

func (f nullEventProcessorFactory) CreateEventProcessor(context interfaces.ClientContext) (ldevents.EventProcessor, error) {
	return ldevents.NewNullEventProcessor(), nil
}
