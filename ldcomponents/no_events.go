package ldcomponents

import (
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

type nullEventProcessorFactory struct{}

// NoEvents returns a configuration object that disables analytics events.
//
// Storing this in the Events field of [github.com/launchdarkly/go-server-sdk/v7.Config] causes the
// SDK to discard all analytics events and not send them to LaunchDarkly, regardless of any other configuration.
//
//	config := ld.Config{
//	    Events: ldcomponents.NoEvents(),
//	}
func NoEvents() subsystems.ComponentConfigurer[ldevents.EventProcessor] {
	return nullEventProcessorFactory{}
}

func (f nullEventProcessorFactory) Build(
	context subsystems.ClientContext,
) (ldevents.EventProcessor, error) {
	return ldevents.NewNullEventProcessor(), nil
}

// This method implements a hidden interface in ldclient_events.go, as a hint to the SDK that this is
// the stub implementation of EventProcessorFactory and therefore LDClient does not need to bother
// generating events at all.
func (f nullEventProcessorFactory) IsNullEventProcessorFactory() bool {
	return true
}
