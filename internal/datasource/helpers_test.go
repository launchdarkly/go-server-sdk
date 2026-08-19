package datasource

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHTTPStatusError(t *testing.T) {
	error := httpStatusError{Message: "message", Code: 500}
	assert.Equal(t, "message", error.Error())
}

// classifyHTTPFailure per RETRY §1.6: 400, 408, 429 are normal (transient
// server-side conditions); all other 4xx are unexpected (durable client-side
// misconfiguration); 5xx and everything else are normal.
func TestClassifyHTTPFailure(t *testing.T) {
	for i := 400; i < 500; i++ {
		expected := FailureClassNormal
		if !(i == 400 || i == 408 || i == 429) {
			expected = FailureClassUnexpected
		}
		assert.Equal(t, expected, classifyHTTPFailure(i), strconv.Itoa(i))
	}
	for i := 500; i < 600; i++ {
		assert.Equal(t, FailureClassNormal, classifyHTTPFailure(i), strconv.Itoa(i))
	}
}

// classifyTransportFailure per RETRY §1.7: TLS/certificate validation failures
// are unexpected; other transport-layer errors are normal.
func TestClassifyTransportFailure(t *testing.T) {
	assert.Equal(t, FailureClassNormal, classifyTransportFailure(nil))
	assert.Equal(t, FailureClassNormal, classifyTransportFailure(errors.New("boom")))

	// TLS certificate errors.
	assert.Equal(t, FailureClassUnexpected,
		classifyTransportFailure(&tls.CertificateVerificationError{}))
	assert.Equal(t, FailureClassUnexpected,
		classifyTransportFailure(x509.UnknownAuthorityError{}))
	assert.Equal(t, FailureClassUnexpected,
		classifyTransportFailure(x509.HostnameError{Host: "example.invalid"}))
	assert.Equal(t, FailureClassUnexpected,
		classifyTransportFailure(x509.CertificateInvalidError{Reason: x509.Expired}))

	// Wrapped errors are still classified correctly.
	wrapped := fmt.Errorf("some wrapper: %w", x509.UnknownAuthorityError{})
	assert.Equal(t, FailureClassUnexpected, classifyTransportFailure(wrapped))
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
