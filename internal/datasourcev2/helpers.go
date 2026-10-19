package datasourcev2

//nolint: godox
// TODO: This was copied from datasource/helpers.go. We should extract these
// out into a common module, or if we decide we don't need these later in the
// v2 implementation, we should clean this up.

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// fdv1FallbackHeader is the response header LaunchDarkly uses to instruct an SDK to abandon the
// FDv2 protocol and fall back to FDv1.
const fdv1FallbackHeader = "X-LD-FD-Fallback"

// isFDv1FallbackRequested reports whether the response headers signal that the SDK should fall
// back to the FDv1 protocol.
func isFDv1FallbackRequested(h http.Header) bool {
	return h.Get(fdv1FallbackHeader) == "true"
}

// fdv1FallbackTTLHeader is the response header that says how many seconds the SDK should stay
// on FDv1 before trying FDv2 again.
const fdv1FallbackTTLHeader = "X-LD-FD-Fallback-TTL"

// fdv1FallbackFromHeaders returns the FDv1 fallback instruction carried by the response headers.
func fdv1FallbackFromHeaders(h http.Header) subsystems.FDv1Fallback {
	if !isFDv1FallbackRequested(h) {
		return subsystems.FDv1Fallback{}
	}
	seconds, err := strconv.ParseInt(h.Get(fdv1FallbackTTLHeader), 10, 64)
	if err != nil {
		return subsystems.FDv1Fallback{Requested: true}
	}
	return subsystems.FDv1Fallback{Requested: true, TTL: fdv1FallbackTTLFromSeconds(seconds)}
}

// fdv1FallbackFromGoodbye returns the FDv1 fallback instruction carried by a goodbye event. A
// goodbye event is an instruction to fall back only if it has a protocolFallbackTTL.
func fdv1FallbackFromGoodbye(goodbye subsystems.Goodbye) subsystems.FDv1Fallback {
	if len(goodbye.ProtocolFallbackTTL) == 0 || string(goodbye.ProtocolFallbackTTL) == "null" {
		return subsystems.FDv1Fallback{}
	}
	var seconds int64
	if err := json.Unmarshal(goodbye.ProtocolFallbackTTL, &seconds); err != nil {
		return subsystems.FDv1Fallback{Requested: true}
	}
	return subsystems.FDv1Fallback{Requested: true, TTL: fdv1FallbackTTLFromSeconds(seconds)}
}

// fdv1FallbackTTLFromSeconds converts a TTL in seconds to a duration. A value that is too large to
// convert gives zero, which means that the TTL is not usable.
func fdv1FallbackTTLFromSeconds(seconds int64) time.Duration {
	if seconds > math.MaxInt64/int64(time.Second) {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

type httpStatusError struct {
	Message string
	Code    int
	Header  http.Header
}

func (e httpStatusError) Error() string {
	return e.Message
}

// Tests whether an HTTP error status represents a condition that might resolve on its own if we retry,
// or at least should not make us permanently stop sending requests.
func isHTTPErrorRecoverable(statusCode int) bool {
	if statusCode >= 400 && statusCode < 500 {
		switch statusCode {
		case 400: // bad request
			return true
		case 408: // request timeout
			return true
		case 429: // too many requests
			return true
		default:
			return false // all other 4xx errors are unrecoverable
		}
	}
	return true
}

func httpErrorDescription(statusCode int) string {
	message := ""
	if statusCode == 401 || statusCode == 403 {
		message = " (invalid SDK key)"
	}
	return fmt.Sprintf("HTTP error %d%s", statusCode, message)
}

// Logs an HTTP error or network error at the appropriate level and determines whether it is recoverable
// (as defined by isHTTPErrorRecoverable).
func checkIfErrorIsRecoverableAndLog(
	loggers ldlog.Loggers,
	errorDesc, errorContext string,
	statusCode int,
	recoverableMessage string,
) bool {
	if statusCode > 0 && !isHTTPErrorRecoverable(statusCode) {
		loggers.Errorf("Error %s (giving up permanently): %s", errorContext, errorDesc)
		return false
	}
	loggers.Warnf("Error %s (%s): %s", errorContext, recoverableMessage, errorDesc)
	return true
}

func checkForHTTPError(statusCode int, header http.Header, url string) error {
	if statusCode == http.StatusUnauthorized {
		return httpStatusError{
			Message: fmt.Sprintf("Invalid SDK key when accessing URL: %s. Verify that your SDK key is correct.", url),
			Code:    statusCode,
			Header:  header,
		}
	}

	if statusCode == http.StatusNotFound {
		return httpStatusError{
			Message: fmt.Sprintf("Resource not found when accessing URL: %s. Verify that this resource exists.", url),
			Code:    statusCode,
			Header:  header,
		}
	}

	if statusCode/100 != 2 {
		return httpStatusError{
			Message: fmt.Sprintf("Unexpected response code: %d when accessing URL: %s", statusCode, url),
			Code:    statusCode,
			Header:  header,
		}
	}
	return nil
}
