package datasource

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/gregjones/httpcache"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// SDK endpoints
const (
	LatestFlagsPath    = "/sdk/latest-flags"
	LatestSegmentsPath = "/sdk/latest-segments"
	LatestAllPath      = "/sdk/latest-all"
)

// requestor is the interface implemented by requestorImpl, used for testing purposes
type requestor interface {
	requestAll() (data allData, cached bool, err error)
}

// requestorImpl is the internal implementation of getting flag/segment data from the LD polling endpoints.
type requestorImpl struct {
	httpClient *http.Client
	baseURI    string
	headers    http.Header
	loggers    ldlog.Loggers
}

type malformedJSONError struct {
	innerError error
}

func (e malformedJSONError) Error() string {
	return e.innerError.Error()
}

func newRequestorImpl(
	context interfaces.ClientContext,
	httpClient *http.Client,
	baseURI string,
) requestor {
	if httpClient == nil {
		httpClient = context.GetHTTP().CreateHTTPClient()
	}

	modifiedClient := *httpClient
	modifiedClient.Transport = &httpcache.Transport{
		Cache:               httpcache.NewMemoryCache(),
		MarkCachedResponses: true,
		Transport:           httpClient.Transport,
	}

	return &requestorImpl{
		httpClient: &modifiedClient,
		baseURI:    baseURI,
		headers:    context.GetHTTP().GetDefaultHeaders(),
		loggers:    context.GetLogging().GetLoggers(),
	}
}

func (r *requestorImpl) requestAll() (allData, bool, error) {
	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	var data allData
	body, cached, err := r.makeRequest(LatestAllPath)
	if err != nil {
		return allData{}, false, err
	}
	if cached {
		return allData{}, true, nil
	}
	jsonErr := json.Unmarshal(body, &data)

	if jsonErr != nil {
		return allData{}, false, malformedJSONError{jsonErr}
	}
	return data, cached, nil
}

func (r *requestorImpl) makeRequest(resource string) ([]byte, bool, error) {
	req, reqErr := http.NewRequest("GET", r.baseURI+resource, nil)
	if reqErr != nil {
		return nil, false, reqErr
	}
	url := req.URL.String()

	for k, vv := range r.headers {
		req.Header[k] = vv
	}

	res, resErr := r.httpClient.Do(req)

	if resErr != nil {
		return nil, false, resErr
	}

	defer func() {
		_, _ = ioutil.ReadAll(res.Body)
		_ = res.Body.Close()
	}()

	if err := checkForHTTPError(res.StatusCode, url); err != nil {
		return nil, false, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := ioutil.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, ioErr // COVERAGE: there is no way to simulate this condition in unit tests
	}
	return body, cached, nil
}
