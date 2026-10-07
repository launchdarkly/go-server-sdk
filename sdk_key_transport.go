package ldclient

import (
	"net/http"
	"sync/atomic"
)

// sdkKeyOverride holds an SDK key that replaces the key that the client was created with.
// It is empty until LDClient.SetSDKKey is called.
type sdkKeyOverride struct {
	key atomic.Pointer[string]
}

func (o *sdkKeyOverride) get() (string, bool) {
	if key := o.key.Load(); key != nil {
		return *key, true
	}
	return "", false
}

func (o *sdkKeyOverride) set(key string) {
	o.key.Store(&key)
}

// sdkKeyTransport replaces the Authorization header of each request with the override key.
//
// The transport replaces the header only when it is present. The HTTP client removes the header
// when it follows a redirect to a different host. Thus the transport does not send the key to
// that host.
type sdkKeyTransport struct {
	base     http.RoundTripper
	override *sdkKeyOverride
}

func (t *sdkKeyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	key, ok := t.override.get()
	if !ok || req.Header.Get("Authorization") == "" {
		return t.base.RoundTrip(req)
	}
	// A RoundTripper must not modify the request that it receives.
	r := req.Clone(req.Context())
	r.Header.Set("Authorization", key)
	return t.base.RoundTrip(r)
}

// wrapHTTPClientFactory returns a factory whose clients send requests through sdkKeyTransport.
func wrapHTTPClientFactory(factory func() *http.Client, override *sdkKeyOverride) func() *http.Client {
	return func() *http.Client {
		client := *factory()
		base := client.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		client.Transport = &sdkKeyTransport{base: base, override: override}
		return &client
	}
}
