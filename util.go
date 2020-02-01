package ldclient

import (
	"fmt"
	"net/http"
	"time"
)

type httpStatusError struct {
	Message string
	Code    int
}

func (e httpStatusError) Error() string {
	return e.Message
}

// unixMillisToUtcTime converts a Unix epoch milliseconds float64 value to the equivalent time.Time value with UTC location
func unixMillisToUtcTime(unixMillis float64) time.Time {
	return time.Unix(0, int64(unixMillis)*int64(time.Millisecond)).UTC()
}

func checkForHttpError(statusCode int, url string) error {
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

// makeAllVersionedDataMap returns a map of version objects grouped by namespace that can be used to initialize a data store
func makeAllVersionedDataMap(
	features map[string]*FeatureFlag,
	segments map[string]*Segment) map[VersionedDataKind]map[string]VersionedData {

	allData := make(map[VersionedDataKind]map[string]VersionedData)
	allData[Features] = make(map[string]VersionedData)
	allData[Segments] = make(map[string]VersionedData)
	for k, v := range features {
		allData[Features][k] = v
	}
	for k, v := range segments {
		allData[Segments][k] = v
	}
	return allData
}

func addBaseHeaders(req *http.Request, sdkKey string, config Config) {
	req.Header.Add("Authorization", sdkKey)
	req.Header.Add("User-Agent", config.UserAgent)
	if config.WrapperName != "" {
		w := config.WrapperName
		if config.WrapperVersion != "" {
			w = w + "/" + config.WrapperVersion
		}
		req.Header.Add("X-LaunchDarkly-Wrapper", w)
	}
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

func httpErrorMessage(statusCode int, context string, recoverableMessage string) string {
	statusDesc := ""
	if statusCode == 401 {
		statusDesc = " (invalid SDK key)"
	}
	resultMessage := recoverableMessage
	if !isHTTPErrorRecoverable(statusCode) {
		resultMessage = "giving up permanently"
	}
	return fmt.Sprintf("Received HTTP error %d%s for %s - %s",
		statusCode, statusDesc, context, resultMessage)
}

func describeUserForErrorLog(key string, logUserKeyInErrors bool) string {
	if logUserKeyInErrors {
		return fmt.Sprintf("user '%s'", key)
	}
	return "a user (enable LogUserKeyInErrors to see the user key)"
}
