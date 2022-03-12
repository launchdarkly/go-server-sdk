package ldservices

import (
	"net/http"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"
)

const (
	// ServerSideSDKStreamingPath is the expected request path for server-side SDK stream requests.
	ServerSideSDKStreamingPath = "/all"
)

// ServerSideStreamingServiceHandler creates an HTTP handler to mimic the LaunchDarkly server-side streaming service.
// It uses httphelpers.SSEHandler(), while also enforcing that the request path is ServerSideSDKStreamingPath and
// that the method is GET.
//
// There must always be an initial event, since LaunchDarkly streams always start with a "put".
//
//     initialData := ldservices.NewServerSDKData().Flags(flag1, flag2) // all clients will get this in a "put" event
//     handler, stream := ldservices.ServerSideStreamingHandler(initialData.ToPutEvent())
//     server := httptest.NewServer(handler)
//     stream.Enqueue(httphelpers.SSEEvent{Event: "patch", Data: myPatchData}) // push an update
//     stream.Close() // force any current stream connections to be closed
func ServerSideStreamingServiceHandler(
	initialEvent httphelpers.SSEEvent,
) (http.Handler, httphelpers.SSEStreamControl) {
	handler, stream := httphelpers.SSEHandler(&initialEvent)
	return httphelpers.HandlerForPath(ServerSideSDKStreamingPath, httphelpers.HandlerForMethod("GET", handler, nil), nil),
		stream
}
