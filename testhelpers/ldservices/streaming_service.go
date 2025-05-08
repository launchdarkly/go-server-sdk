package ldservices

import (
	"net/http"

	"github.com/launchdarkly/go-server-sdk/v7/testhelpers/ldservicesv2"

	"github.com/launchdarkly/go-test-helpers/v3/httphelpers"
)

const (
	// ServerSideSDKStreamingPath is the expected request path for server-side SDK stream requests.
	ServerSideSDKStreamingPath = "/all"

	// ServerSideSDKV2StreamingPath is the expected request path for server-side SDK stream requests for v2.
	ServerSideSDKV2StreamingPath = "/sdk/stream"
)

// ServerSideStreamingServiceHandler creates an HTTP handler to mimic the LaunchDarkly server-side streaming service.
// It uses httphelpers.SSEHandler(), while also enforcing that the request path is ServerSideSDKStreamingPath and
// that the method is GET.
//
// There must always be an initial event, since LaunchDarkly streams always start with a "put".
//
//	initialData := ldservices.NewServerSDKData().Flags(flag1, flag2) // all clients will get this in a "put" event
//	handler, stream := ldservices.ServerSideStreamingHandler(initialData.ToPutEvent())
//	server := httptest.NewServer(handler)
//	stream.Enqueue(httphelpers.SSEEvent{Event: "patch", Data: myPatchData}) // push an update
//	stream.Close() // force any current stream connections to be closed
func ServerSideStreamingServiceHandler(
	initialEvent httphelpers.SSEEvent,
) (http.Handler, httphelpers.SSEStreamControl) {
	return serverSideStreamingHandler(initialEvent, ServerSideSDKStreamingPath)
}

// ServerSideStreamingV2ServiceProtocolHandler creates an HTTP handler to mimic
// the LaunchDarkly server-side streaming service. It uses
// httphelpers.SSEHandler(), while also enforcing that the request path is
// ServerSideSDKStreamingPath and that the method is GET.
//
//	initialData := ldservicesv2.NewServerSDKData().Flags(flag1, flag2)
//	protocol := ldservicesv2.NewStreamingProtocol().
//		WithIntent(subsystems.ServerIntent{Payload: subsystems.Payload{
//			ID: "fake-id", Target: 0, Code: "xfer-full", Reason: "payload-missing",
//		}}).
//	WithPutObjects(initialData.ToPutObjects()).
//	WithTransferred(1)
//	handler, stream := ldservices.ServerSideStreamingV2ServiceProtocolHandler(protocol)
//	server := httptest.NewServer(handler)
//
//	protocol.WithPutObject(subsystems.PutObject{
//		Version: 10,
//		Kind:    subsystems.FlagKind,
//		Key:     alwaysTrueFlag.Key,
//		Object:  jsonFlag,
//	}
//	protocol.WithTransferred(1)
//
//	protocol.Enqueue(stream) // push all updates
//	stream.Close() // force any current stream connections to be closed
func ServerSideStreamingV2ServiceProtocolHandler(
	protocol *ldservicesv2.StreamingProtocol,
) (http.Handler, httphelpers.SSEStreamControl) {
	handler, stream := httphelpers.SSEHandler(nil)
	protocol.Enqueue(stream)

	return httphelpers.HandlerForPath(
		ServerSideSDKV2StreamingPath,
		httphelpers.HandlerForMethod("GET", handler, nil),
		nil,
	), stream
}

func serverSideStreamingHandler(
	initialEvent httphelpers.SSEEvent, path string,
) (http.Handler, httphelpers.SSEStreamControl) {
	handler, stream := httphelpers.SSEHandler(&initialEvent)
	return httphelpers.HandlerForPath(path, httphelpers.HandlerForMethod("GET", handler, nil), nil),
		stream
}
