package ldservices

import (
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/launchdarkly/eventsource"
	"github.com/launchdarkly/go-test-helpers/httphelpers"
)

const serverSideSDKStreamingPath = "/all"

// ServerSideStreamingServiceHandler creates an HTTP handler to mimic the LaunchDarkly server-side streaming service.
//
// If initialEvent is non-nil, it will be sent at the beginning of each connection; you can pass a *ServerSDKData value
// to generate a "put" event.
//
// Any events that are pushed to eventsCh (if it is non-nil) will be published to the stream.
//
// Calling Close() on the returned io.Closer causes the handler to close any active stream connections and refuse all
// subsequent requests. You don't need to do this unless you need to force a stream to disconnect before the test
// server has been shut down; shutting down the server will close connections anyway.
//
//     initialData := ldservices.NewSDKData().Flags(flag1, flag2) // all connections will receive this in a "put" event
//     eventsCh := make(chan eventsource.Event)
//     handler, closer := ldservices.ServerSideStreamingHandler(initialData, eventsCh)
//     server := httptest.NewServer(handler)
//     eventsCh <- ldservices.NewSSEEvent("", "patch", myPatchData) // push a "patch" event
//     closer.Close() // force any current stream connections to be closed
func ServerSideStreamingServiceHandler(
	initialEvent eventsource.Event,
	eventsCh <-chan eventsource.Event,
) (http.Handler, io.Closer) {
	closerCh := make(chan struct{})
	sh := &serverSideStreamingServiceHandler{
		initialEvent: initialEvent,
		eventsCh:     eventsCh,
		closerCh:     closerCh,
	}
	h := httphelpers.HandlerForPath(serverSideSDKStreamingPath, httphelpers.HandlerForMethod("GET", sh, nil), nil)
	c := &serverSideStreamingServiceCloser{
		closerCh: closerCh,
	}
	return h, c
}

type serverSideStreamingServiceHandler struct {
	initialEvent eventsource.Event
	eventsCh     <-chan eventsource.Event
	closed       bool
	closerCh     <-chan struct{}
}

type serverSideStreamingServiceCloser struct {
	closerCh  chan<- struct{}
	closeOnce sync.Once
}

func (s *serverSideStreamingServiceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("ServerSideStreamingServiceHandler can't be used with a ResponseWriter that does not support Flush")
		w.WriteHeader(500)
		return
	}
	if s.closed {
		log.Println("ServerSideStreamingServiceHandler received a request after it was closed")
		w.WriteHeader(500)
		return
	}

	// Note that we're not using eventsource.Server to provide a streamed response, because eventsource doesn't
	// have a mechanism for forcing the server to drop the connection while the client is still waiting, and
	// that's a condition we want to be able to simulate in tests.

	var closeNotifyCh <-chan bool
	// CloseNotifier is deprecated but there's no way to use Context in this case
	if closeNotifier, ok := w.(http.CloseNotifier); ok { //nolint:megacheck
		closeNotifyCh = closeNotifier.CloseNotify()
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-cache, no-store, must-revalidate")

	encoder := eventsource.NewEncoder(w, false)

	if s.initialEvent != nil {
		_ = encoder.Encode(s.initialEvent)
	}
	flusher.Flush()

StreamLoop:
	for {
		select {
		case e := <-s.eventsCh:
			_ = encoder.Encode(e)
			flusher.Flush()
		case <-s.closerCh:
			s.closed = true
			break StreamLoop
		case <-closeNotifyCh:
			// client has closed the connection
			break StreamLoop
		}
	}
}

func (c *serverSideStreamingServiceCloser) Close() error {
	c.closeOnce.Do(func() {
		close(c.closerCh)
	})
	return nil
}
