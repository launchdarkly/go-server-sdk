package datasource

import (
	"fmt"
	"net/http"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	st "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/ldstoretypes"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"

	"gopkg.in/launchdarkly/go-jsonstream.v1/jreader"
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

// This method parses a JSON data structure representing a full set of SDK data. For example:
//
// {
//   "flags": {
//     "flag1": { "key": "flag1", "version": 1, ...etc. },
//     "flag2": { "key": "flag2", "version": 1, ...etc. },
//   },
//   "segments": {
//     "segment1": { "key", "segment1", "version": 1, ...etc. }
//   }
// }
//
// Even though this is map-like, we don't return the data as a map, because the SDK does not need to
// manipulate it as a map. Our data store API instead expects a list of Collections, each of which has
// a list of data items, so that's what we build here.
//
// This representation makes up the entirety of a polling response for PollingDataSource, and is a
// subset of the stream data for StreamingDataSource.
func parseAllStoreDataFromJSONReader(r *jreader.Reader) []st.Collection {
	var ret []st.Collection
	for dataObj := r.Object(); dataObj.Next(); {
		var dataKind datakinds.DataKindInternal
		switch string(dataObj.Name()) {
		case "flags":
			dataKind = datakinds.Features
		case "segments":
			dataKind = datakinds.Segments
		default: // unrecognized category, skip it
			continue
		}
		coll := st.Collection{Kind: dataKind}
		for keysToItemsObj := r.Object(); keysToItemsObj.Next(); {
			key := string(keysToItemsObj.Name())
			item, err := dataKind.DeserializeFromJSONReader(r)
			if err == nil {
				coll.Items = append(coll.Items, st.KeyedItemDescriptor{Key: key, Item: item})
			}
		}
		ret = append(ret, coll)
	}
	return ret
}
