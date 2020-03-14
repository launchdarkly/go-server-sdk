package shared_test

import (
	"fmt"
	"net/http"

	"github.com/launchdarkly/eventsource"
)

type testSSEEvent struct {
	id, event, data string
}

func (e testSSEEvent) Id() string    { return e.id }
func (e testSSEEvent) Event() string { return e.event }
func (e testSSEEvent) Data() string  { return e.data }

// NewSSEEvent constructs an implementation of eventsource.Event.
func NewSSEEvent(id, event, data string) eventsource.Event {
	return testSSEEvent{id, event, data}
}

type testSSERepo struct {
	initialEvent eventsource.Event
}

func (r *testSSERepo) Replay(channel, id string) chan eventsource.Event {
	c := make(chan eventsource.Event, 1)
	c <- r.initialEvent
	return c
}

// SDKData is a struct that, when converted to a string, provides the JSON encoding of a payload of flags/segments.
//
// The JSON objects representing the flags and segments maps must be pre-serialized, because this package cannot
// refer back to the main package where FeatureFlag and Segment are defined.
//
// SDKData also implements EventSource.Event; it will behave like a "put" event with the same data.
type SDKData struct {
	FlagsData    []byte
	SegmentsData []byte
}

// String produces a JSON string representation of the data.
func (s SDKData) String() string {
	maybeJSON := func(bytes []byte) string {
		if len(bytes) == 0 {
			return "{}"
		}
		return string(bytes)
	}
	return `{"flags":` + maybeJSON(s.FlagsData) + `,"segments":` + maybeJSON(s.SegmentsData) + "}"
}

// Id is part of SDKData's implementation of eventsource.Event.
func (s *SDKData) Id() string { return "" }

// Event is part of SDKData's implementation of eventsource.Event.
func (s *SDKData) Event() string { return "put" }

// Data is part of SDKData's implementation of eventsource.Event.
func (s *SDKData) Data() string { return `{"path": "/", "data": ` + s.String() + "}" }

// NewStreamingServiceHandler creates an HTTP handler mimicking the streaming service. It provides initialData
// (presumably a "put" event) immediately, and then publishes any events that are pushed to eventsCh.
func NewStreamingServiceHandler(initialData eventsource.Event, eventsCh <-chan eventsource.Event) http.Handler {
	repo := &testSSERepo{}
	if initialData != nil {
		repo.initialEvent = initialData
	}
	channelID := "test"

	esserver := eventsource.NewServer()
	esserver.ReplayAll = true
	esserver.Register(channelID, repo)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/all" {
			if eventsCh != nil {
				go func() {
					for e := range eventsCh {
						esserver.Publish([]string{channelID}, e)
					}
				}()
			}
			esserver.Handler(channelID).ServeHTTP(w, r)
		} else {
			w.WriteHeader(404)
		}
	})
}

// NewPollingServiceHandler creates an HTTP handler mimicking the polling service.
func NewPollingServiceHandler(data fmt.Stringer) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdk/latest-all" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_, _ = w.Write([]byte(data.String()))
		} else {
			w.WriteHeader(404)
		}
	})
}

// NewEventsServiceHandler creates an HTTP handler mimicking the events service.
func NewEventsServiceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bulk" || r.URL.Path == "/diagnostic" {
			w.WriteHeader(202)
		} else {
			w.WriteHeader(404)
		}
	})
}
