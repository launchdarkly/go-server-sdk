package shared_test

import (
	"io/ioutil"
	"net/http"
)

// HTTPRequestInfo represents a request captured by NewRecordingHTTPHandler.
type HTTPRequestInfo struct {
	Request *http.Request
	Body    []byte // body has to be captured separately by the test server because you can't read it after the response is sent
}

func getRequestBody(request *http.Request) []byte {
	body, _ := ioutil.ReadAll(request.Body)
	return body
}

// NewRecordingHTTPHandler wraps an HTTP handler in another handler that pushes received requests onto a channel.
func NewRecordingHTTPHandler(delegateToHandler http.Handler) (http.Handler, <-chan HTTPRequestInfo) {
	requestsCh := make(chan HTTPRequestInfo, 100)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCh <- HTTPRequestInfo{r, getRequestBody(r)}
		delegateToHandler.ServeHTTP(w, r)
	})
	return handler, requestsCh
}

// NewHTTPHandlerReturningStatus creates an HTTP handler that always returns the same status code.
func NewHTTPHandlerReturningStatus(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
}
