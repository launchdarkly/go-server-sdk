package httphelpers

import (
	"log"
	"net/http"
	"sync"
)

// StreamControl is the interface for manipulating streams created by ChunkedStreamingHandler.
type StreamControl interface {
	// Enqueue is the same as Send, except that if there are currently no open connections to this
	// endpoint, the data is enqueued and will be sent to the next client that connects.
	Enqueue(data []byte)

	// Send sends a chunk of data. If there are multiple open connections to this endpoint, the same
	// data is sent to all of them. If there are no open connections, the data is discarded.
	Send(data []byte)

	// EndAll terminates any existing connections to this endpoint, but allows new connections
	// afterward.
	EndAll()

	// Close terminates any existing connections to this endpoint and causes the handler to reject any
	// subsequent connections.
	Close() error
}

// ChunkedStreamingHandler creates an HTTP handler that streams arbitrary data using chunked encoding.
//
// The initialData parameter, if not nil, specifies a starting chunk that should always be sent to any
// client that has connected to this endpoint. Then, any data provided via the StreamControl interface
// is copied to all connected clients. Connections remain open until either EndAll or Close is called
// on the StreamControl.
//
// In this example, every request to this endpoint will receive an initial message of "hello\n", and
// then another line will be sent every second with a counter; every 30 seconds, all active stream
// connections are closed:
//
//	handler, stream := httphelpers.ChunkedStreamingHandler([]byte("hello\n"), "text/plain")
//	(start server with handler)
//	go func() {
//	    n := 1
//	    counter := time.NewTicker(time.Second)
//	    interrupter := time.NewTicker(time.Second * 10)
//	    for {
//	        select {
//	        case <-counter.C:
//	            stream.Send([]byte(fmt.Sprintf("%d\n", n)))
//	            n++
//	        case <-interrupter.C:
//	            stream.EndAll()
//	        }
//	    }
//	}()
func ChunkedStreamingHandler(initialChunk []byte, contentType string) (http.Handler, StreamControl) {
	sh := &chunkedStreamingHandlerImpl{
		initialChunk: initialChunk,
		contentType:  contentType,
	}
	return sh, sh
}

type chunkedStreamingHandlerImpl struct {
	initialChunk []byte
	contentType  string
	queued       [][]byte
	channels     []chan []byte
	closed       bool
	lock         sync.Mutex
}

func (s *chunkedStreamingHandlerImpl) Enqueue(data []byte) {
	s.sendInternal(data, true)
}

func (s *chunkedStreamingHandlerImpl) Send(data []byte) {
	s.sendInternal(data, false)
}

func (s *chunkedStreamingHandlerImpl) EndAll() {
	s.endAllInternal(false)
}

func (s *chunkedStreamingHandlerImpl) Close() error {
	s.endAllInternal(true)
	return nil
}

func (s *chunkedStreamingHandlerImpl) sendInternal(data []byte, enqueueIfNoChannels bool) {
	if len(data) == 0 {
		// In chunked encoding, a zero-length chunk terminates the response. We don't want the caller to
		// do that by accident, so we require that they call EndAll or Close instead.
		return
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if s.closed {
		return
	}

	if len(s.channels) == 0 {
		if enqueueIfNoChannels {
			s.queued = append(s.queued, data)
		}
		return
	}

	for _, ch := range s.channels {
		ch <- data
	}
}

func (s *chunkedStreamingHandlerImpl) endAllInternal(thenClose bool) {
	s.lock.Lock()
	if thenClose {
		s.closed = true
	}
	channels := s.channels
	s.channels = nil
	s.lock.Unlock()

	for _, ch := range channels {
		close(ch)
	}
}

func (s *chunkedStreamingHandlerImpl) removeChannel(channelToRemove chan []byte) {
	// This is called when the client closed the connection.
	go func() {
		// Consume anything else that gets sent on this channel, until it's closed, to avoid deadlock
		for range channelToRemove {
		}
	}()

	s.lock.Lock()
	for i, ch := range s.channels {
		if ch == channelToRemove {
			copy(s.channels[i:], s.channels[i+1:])
			s.channels[len(s.channels)-1] = nil
			s.channels = s.channels[:len(s.channels)-1]
			break
		}
	}
	s.lock.Unlock()

	// At this point, no one else will ever see this channel, so it's safe to close
	close(channelToRemove)
}

func (s *chunkedStreamingHandlerImpl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("httphelpers.ChunkedStreamingHandler can't be used with a ResponseWriter that does not support Flush")
		w.WriteHeader(500)
		return
	}

	s.lock.Lock()
	if s.closed {
		log.Println("httphelpers.ChunkedStreamingHandler received a request after it was closed")
		w.WriteHeader(500)
		s.lock.Unlock()
		return
	}
	dataCh := make(chan []byte, 10)
	s.channels = append(s.channels, dataCh)
	queued := s.queued
	s.queued = nil
	s.lock.Unlock()

	h := w.Header()
	h.Set("Content-Type", s.contentType)
	h.Set("Cache-Control", "no-cache, no-store, must-revalidate")

	if s.initialChunk != nil {
		_, _ = w.Write(s.initialChunk)
		flusher.Flush()
	}

	for _, data := range queued {
		_, _ = w.Write(data)
		flusher.Flush()
	}

	flusher.Flush()

	var closeNotifyCh <-chan bool
	// CloseNotifier is deprecated but there's no way to use Context in this case
	if closeNotifier, ok := w.(http.CloseNotifier); ok { //nolint:megacheck
		closeNotifyCh = closeNotifier.CloseNotify()
	}

StreamLoop:
	for {
		select {
		case data, ok := <-dataCh:
			if !ok { // closed
				break StreamLoop
			}
			_, _ = w.Write(data)
			flusher.Flush()
		case <-closeNotifyCh:
			// client has closed the connection
			s.removeChannel(dataCh)
			break StreamLoop
		}
	}
}
