package shared_test

import (
	"net/http"

	"github.com/launchdarkly/eventsource"
)

type testSSEEvent struct {
	id, event, data string
}

func (e testSSEEvent) Id() string    { return e.id }
func (e testSSEEvent) Event() string { return e.event }
func (e testSSEEvent) Data() string  { return e.data }

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

type SDKData struct {
	FlagsData    []byte
	SegmentsData []byte
}

func (s SDKData) String() string {
	maybeJSON := func(bytes []byte) string {
		if len(bytes) == 0 {
			return "{}"
		}
		return string(bytes)
	}
	return `{"flags":` + maybeJSON(s.FlagsData) + `,"segments":` + maybeJSON(s.SegmentsData) + "}"
}

func (s *SDKData) Id() string    { return "" }
func (s *SDKData) Event() string { return "put" }
func (s *SDKData) Data() string  { return `{"path": "/", "data": ` + s.String() + "}" }

func NewStreamingServiceHandler(initialData *SDKData, eventsCh <-chan eventsource.Event) http.Handler {
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

func NewPollingServiceHandler(data SDKData) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sdk/latest-all" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			w.Write([]byte(data.String()))
		} else {
			w.WriteHeader(404)
		}
	})
}

func NewEventsServiceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bulk" || r.URL.Path == "/diagnostic" {
			w.WriteHeader(202)
		} else {
			w.WriteHeader(404)
		}
	})
}
