package ldclient

import (
	"fmt"
	"net/http"

	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type httpStatusError struct {
	Message string
	Code    int
}

func (e httpStatusError) Error() string {
	return e.Message
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
	features map[string]*ldmodel.FeatureFlag,
	segments map[string]*ldmodel.Segment) map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData {

	allData := make(map[interfaces.VersionedDataKind]map[string]interfaces.VersionedData)
	allData[interfaces.DataKindFeatures()] = make(map[string]interfaces.VersionedData)
	allData[interfaces.DataKindSegments()] = make(map[string]interfaces.VersionedData)
	for k, v := range features {
		allData[interfaces.DataKindFeatures()][k] = v
	}
	for k, v := range segments {
		allData[interfaces.DataKindSegments()][k] = v
	}
	return allData
}

func addBaseHeaders(h http.Header, sdkKey string, config Config) {
	h.Add("Authorization", sdkKey)
	h.Add("User-Agent", config.UserAgent)
	if config.WrapperName != "" {
		w := config.WrapperName
		if config.WrapperVersion != "" {
			w = w + "/" + config.WrapperVersion
		}
		h.Add("X-LaunchDarkly-Wrapper", w)
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
