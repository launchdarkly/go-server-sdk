package ldclient

import ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"

type eventsAccumulator struct {
	summarizer           ldevents.EventSummarizer
	featureRequestEvents []ldevents.FeatureRequestEvent
	identifyEvents       []ldevents.IdentifyEvent
	customEvents         []ldevents.CustomEvent
}
