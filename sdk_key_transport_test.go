package ldclient

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingRoundTripper struct {
	requests []*http.Request
}

func (r *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	r.requests = append(r.requests, req)
	return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
}

func newAuthorizedRequest(t *testing.T, key string) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", key)
	return req
}

func TestSDKKeyTransportSendsOriginalKeyWithoutOverride(t *testing.T) {
	base := &recordingRoundTripper{}
	transport := &sdkKeyTransport{base: base, override: &sdkKeyOverride{}}

	// Send a request before any override is set.
	_, err := transport.RoundTrip(newAuthorizedRequest(t, "original-key"))
	require.NoError(t, err)

	// The request should keep its original key.
	require.Len(t, base.requests, 1)
	assert.Equal(t, "original-key", base.requests[0].Header.Get("Authorization"))
}

func TestSDKKeyTransportReplacesKeyWithOverride(t *testing.T) {
	base := &recordingRoundTripper{}
	override := &sdkKeyOverride{}
	override.set("new-key")
	transport := &sdkKeyTransport{base: base, override: override}
	req := newAuthorizedRequest(t, "original-key")

	// Send a request after the override is set.
	_, err := transport.RoundTrip(req)
	require.NoError(t, err)

	// The sent request should carry the new key, and the caller's request should not change.
	require.Len(t, base.requests, 1)
	assert.Equal(t, "new-key", base.requests[0].Header.Get("Authorization"))
	assert.Equal(t, "original-key", req.Header.Get("Authorization"))
}

func TestSDKKeyTransportDoesNotAddKeyToRequestWithoutAuthorization(t *testing.T) {
	base := &recordingRoundTripper{}
	override := &sdkKeyOverride{}
	override.set("new-key")
	transport := &sdkKeyTransport{base: base, override: override}
	req, err := http.NewRequest(http.MethodGet, "http://localhost/", nil)
	require.NoError(t, err)

	// Send a request that has no Authorization header.
	_, err = transport.RoundTrip(req)
	require.NoError(t, err)

	// The sent request should still have no Authorization header.
	require.Len(t, base.requests, 1)
	assert.Empty(t, base.requests[0].Header.Values("Authorization"))
}

func TestWrappedHTTPClientFactoryKeepsClientSettings(t *testing.T) {
	base := &recordingRoundTripper{}
	factory := func() *http.Client { return &http.Client{Transport: base, Timeout: 42} }
	override := &sdkKeyOverride{}
	override.set("new-key")

	// Create a client from the wrapped factory and send a request.
	client := wrapHTTPClientFactory(factory, override)()
	resp, err := client.Do(newAuthorizedRequest(t, "original-key"))
	require.NoError(t, err)
	_ = resp.Body.Close()

	// The client should keep its timeout and send the request through the original transport with the new key.
	assert.Equal(t, factory().Timeout, client.Timeout)
	require.Len(t, base.requests, 1)
	assert.Equal(t, "new-key", base.requests[0].Header.Get("Authorization"))
}
