package datasourcev2

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPStatusError(t *testing.T) {
	error := httpStatusError{Message: "message", Code: 500}
	assert.Equal(t, "message", error.Error())
}

func TestIsHTTPErrorRecoverable(t *testing.T) {
	for i := 400; i < 500; i++ {
		assert.Equal(t, i == 400 || i == 408 || i == 429, isHTTPErrorRecoverable(i), strconv.Itoa(i))
	}
	for i := 500; i < 600; i++ {
		assert.True(t, isHTTPErrorRecoverable(i))
	}
}

func TestHTTPErrorDescription(t *testing.T) {
	assert.Equal(t, "HTTP error 400", httpErrorDescription(400))
	assert.Equal(t, "HTTP error 401 (invalid SDK key)", httpErrorDescription(401))
	assert.Equal(t, "HTTP error 403 (invalid SDK key)", httpErrorDescription(403))
	assert.Equal(t, "HTTP error 500", httpErrorDescription(500))
}

// filterTest represents the expected URL query parameter that should
// be generated for a particular filter key. For example, filter 'foo' should generate
// query parameter 'filter=foo'.
type filterTest struct {
	key   string
	query string
}

// testWithFilters generates a nested test for a set of relevant filters.
// The 'test' function is executed with the requested filter, and the expected query parameter
// for that filter.
func testWithFilters(t *testing.T, test func(t *testing.T, filterTest filterTest)) {
	testCases := map[string]filterTest{
		"no filter":                   {"", ""},
		"filter requires no encoding": {"microservice-1", "filter=microservice-1"},
		"filter requires urlencoding": {"micro service 1", "filter=micro+service+1"},
	}
	for name, params := range testCases {
		t.Run(name, func(t *testing.T) {
			test(t, params)
		})
	}
}

func TestFDv1FallbackFromHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fallback string
		ttl      string
		want     *time.Duration
	}{
		{name: "no fallback header", ttl: "120", want: nil},
		{name: "no TTL header", fallback: "true", want: new(time.Duration)},
		{name: "valid TTL", fallback: "true", ttl: "120", want: ttlPointer(120 * time.Second)},
		{name: "malformed TTL", fallback: "true", ttl: "soon", want: new(time.Duration)},
		{name: "fractional TTL", fallback: "true", ttl: "1.5", want: new(time.Duration)},
		{name: "negative TTL", fallback: "true", ttl: "-5", want: ttlPointer(-5 * time.Second)},
		{name: "huge TTL", fallback: "true", ttl: "9223372036854775807", want: new(time.Duration)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.fallback != "" {
				h.Set(fdv1FallbackHeader, tc.fallback)
			}
			if tc.ttl != "" {
				h.Set(fdv1FallbackTTLHeader, tc.ttl)
			}
			assert.Equal(t, tc.want, fdv1FallbackFromHeaders(h))
		})
	}
}

func TestFDv1FallbackFromGoodbye(t *testing.T) {
	for _, tc := range []struct {
		name string
		ttl  string
		want *time.Duration
	}{
		{name: "no TTL", want: nil},
		{name: "null TTL", ttl: "null", want: nil},
		{name: "valid TTL", ttl: "120", want: ttlPointer(120 * time.Second)},
		{name: "zero TTL", ttl: "0", want: new(time.Duration)},
		{name: "malformed TTL", ttl: `"soon"`, want: new(time.Duration)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			goodbye := subsystems.Goodbye{Reason: "test", ProtocolFallbackTTL: json.RawMessage(tc.ttl)}
			assert.Equal(t, tc.want, fdv1FallbackFromGoodbye(goodbye))
		})
	}
}

func TestGoodbyeUnmarshalsProtocolFallbackTTL(t *testing.T) {
	var goodbye subsystems.Goodbye
	require.NoError(t, json.Unmarshal([]byte(`{"reason":"r","protocolFallbackTTL":120}`), &goodbye))

	assert.Equal(t, ttlPointer(120*time.Second), fdv1FallbackFromGoodbye(goodbye))
}
