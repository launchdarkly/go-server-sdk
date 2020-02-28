package shared_test

import (
	"io/ioutil"
	"net/http"
)

type HTTPRequestInfo struct {
	Request *http.Request
	Body    []byte // body has to be captured separately by the test server because you can't read it after the response is sent
}

func GetRequestBody(request *http.Request) []byte {
	body, _ := ioutil.ReadAll(request.Body)
	return body
}

func NewRecordingHTTPHandler(delegateToHandler http.Handler) (http.Handler, <-chan HTTPRequestInfo) {
	requestsCh := make(chan HTTPRequestInfo, 100)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsCh <- HTTPRequestInfo{r, GetRequestBody(r)}
		delegateToHandler.ServeHTTP(w, r)
	})
	return handler, requestsCh
}

func NewHTTPHandlerReturningStatus(status int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	})
}
