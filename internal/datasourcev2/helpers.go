package datasourcev2

//nolint: godox
// TODO: This was copied from datasource/helpers.go. We should extract these
// out into a common module, or if we decide we don't need these later in the
// v2 implementation, we should clean this up.

import (
	"fmt"
	"net/http"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
)

type httpStatusError struct {
	Message string
	Code    int
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

func checkForHTTPError(statusCode int, url string) error {
	if statusCode == http.StatusUnauthorized {
		return httpStatusError{
			Message: fmt.Sprintf("Invalid SDK key when accessing URL: %s. Verify that your SDK key is correct.", url),
			Code:    statusCode}
	}

	if statusCode == http.StatusNotFound {
		return httpStatusError{
			Message: fmt.Sprintf("Resource not found when accessing URL: %s. Verify that this resource exists.", url),
			Code:    statusCode}
	}

	if statusCode/100 != 2 {
		return httpStatusError{
			Message: fmt.Sprintf("Unexpected response code: %d when accessing URL: %s", statusCode, url),
			Code:    statusCode}
	}
	return nil
}
