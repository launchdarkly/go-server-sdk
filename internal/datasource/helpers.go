package datasource

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
)

type httpStatusError struct {
	Message string
	Code    int
}

func (e httpStatusError) Error() string {
	return e.Message
}

// FailureClass categorizes a data source failure per RETRY §1.5--§1.7. Under the
// RETRY spec no failure is permanently terminal: every failure is either "normal"
// (regular backoff and retry) or "unexpected" (extended backoff via a longer
// retry profile or wait interval, still retrying indefinitely).
type FailureClass int

const (
	// FailureClassNormal indicates a failure that a caller should treat as an
	// ordinary transient error.
	FailureClassNormal FailureClass = iota
	// FailureClassUnexpected indicates a failure that the caller should treat as
	// signalling a durable, non-transient upstream problem.
	FailureClassUnexpected
)

// classifyHTTPFailure returns the failure classification for an HTTP status code
// received during a data source request, per RETRY §1.6. Called only when the
// status indicates failure (non-2xx).
func classifyHTTPFailure(statusCode int) FailureClass {
	if statusCode >= 400 && statusCode < 500 {
		switch statusCode {
		case 400, 408, 429:
			return FailureClassNormal
		default:
			return FailureClassUnexpected
		}
	}
	return FailureClassNormal
}

// classifyTransportFailure returns the failure classification for a transport-layer
// error (i.e., not an HTTP response, but a lower-level network or TLS failure)
// per RETRY §1.7. TLS/certificate validation failures are treated as unexpected;
// everything else is treated as normal.
func classifyTransportFailure(err error) FailureClass {
	if err == nil {
		return FailureClassNormal
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return FailureClassUnexpected
	}
	var x509UnknownAuthorityErr x509.UnknownAuthorityError
	if errors.As(err, &x509UnknownAuthorityErr) {
		return FailureClassUnexpected
	}
	var x509HostnameErr x509.HostnameError
	if errors.As(err, &x509HostnameErr) {
		return FailureClassUnexpected
	}
	var x509InvalidErr x509.CertificateInvalidError
	if errors.As(err, &x509InvalidErr) {
		return FailureClassUnexpected
	}
	return FailureClassNormal
}

func httpErrorDescription(statusCode int) string {
	message := ""
	if statusCode == 401 || statusCode == 403 {
		message = " (authentication failed)"
	}
	return fmt.Sprintf("HTTP error %d%s", statusCode, message)
}

// classifyAndLogHTTPFailure classifies an HTTP failure per RETRY §1.6, logs it,
// and returns the classification for the caller to act on.
func classifyAndLogHTTPFailure(
	loggers ldlog.Loggers,
	errorDesc, errorContext string,
	statusCode int,
	willRetryMessage string,
) FailureClass {
	loggers.Warnf("Error %s (%s): %s", errorContext, willRetryMessage, errorDesc)
	return classifyHTTPFailure(statusCode)
}

// classifyAndLogTransportFailure classifies a transport-layer failure per RETRY
// §1.7, logs it, and returns the classification.
func classifyAndLogTransportFailure(
	loggers ldlog.Loggers,
	err error,
	errorContext, willRetryMessage string,
) FailureClass {
	loggers.Warnf("Error %s (%s): %s", errorContext, willRetryMessage, err.Error())
	return classifyTransportFailure(err)
}

func checkForHTTPError(statusCode int, url string) error {
	if statusCode == http.StatusUnauthorized {
		return httpStatusError{
			Message: fmt.Sprintf("Authentication failed for URL: %s. If this persists, verify that your SDK key is correct.",
				url),
			Code: statusCode}
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
//	{
//	  "flags": {
//	    "flag1": { "key": "flag1", "version": 1, ...etc. },
//	    "flag2": { "key": "flag2", "version": 1, ...etc. },
//	  },
//	  "segments": {
//	    "segment1": { "key", "segment1", "version": 1, ...etc. }
//	  }
//	}
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
