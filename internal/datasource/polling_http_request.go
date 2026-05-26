package datasource

import (
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/launchdarkly/go-sdk-common/v4/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-jsonstream/v4/jreader"

	"maps"

	"github.com/gregjones/httpcache"
)

// PollingRequester is the internal implementation of getting flag/segment data from the LD polling endpoints.
type PollingRequester struct {
	httpClient *http.Client
	baseURI    string
	filterKey  string
	headers    http.Header
	loggers    ldlog.Loggers
}

type malformedJSONError struct {
	innerError error
}

func (e malformedJSONError) Error() string {
	return e.innerError.Error()
}

// NewPollingRequester creates a new PollingRequester instance. This is for
// internal use only and should not be relied upon by consumers.
func NewPollingRequester(
	context subsystems.ClientContext,
	httpClient *http.Client,
	baseURI string,
	filterKey string,
) *PollingRequester {
	if httpClient == nil {
		httpClient = context.GetHTTP().CreateHTTPClient()
	}

	modifiedClient := *httpClient
	modifiedClient.Transport = &httpcache.Transport{
		Cache:               httpcache.NewMemoryCache(),
		MarkCachedResponses: true,
		Transport:           httpClient.Transport,
	}

	return &PollingRequester{
		httpClient: &modifiedClient,
		baseURI:    baseURI,
		filterKey:  filterKey,
		headers:    context.GetHTTP().DefaultHeaders,
		loggers:    context.GetLogging().Loggers,
	}
}

// BaseURI returns the base URI used for the polling request.
func (r *PollingRequester) BaseURI() string {
	return r.baseURI
}

// FilterKey returns the filter key used for the polling request.
func (r *PollingRequester) FilterKey() string {
	return r.filterKey
}

// Request makes a request to the LaunchDarkly polling endpoint for feature flag updates.
// It returns the data in the form of a slice of ldstoretypes.Collection.
// The boolean return value indicates whether the data was served from the cache.
// If an error occurs, it will be returned as the third return value.
func (r *PollingRequester) Request() ([]ldstoretypes.Collection, bool, http.Header, error) {
	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	body, cached, headers, err := r.makeRequest(endpoints.PollingRequestPath)
	if err != nil {
		return nil, false, headers, err
	}
	if cached {
		return nil, true, headers, nil
	}

	reader := jreader.NewReader(body)
	data := parseAllStoreDataFromJSONReader(&reader)
	if err := reader.Error(); err != nil {
		return nil, false, headers, malformedJSONError{err}
	}
	return data, cached, headers, nil
}

func (r *PollingRequester) makeRequest(resource string) ([]byte, bool, http.Header, error) {
	req, reqErr := http.NewRequest("GET", endpoints.AddPath(r.baseURI, resource), nil)
	if reqErr != nil {
		reqErr = fmt.Errorf(
			"unable to create a poll request; this is not a network problem, most likely a bad base URI: %w",
			reqErr,
		)
		return nil, false, nil, reqErr
	}
	if r.filterKey != "" {
		req.URL.RawQuery = url.Values{
			"filter": {r.filterKey},
		}.Encode()
	}
	url := req.URL.String()
	if r.headers != nil {
		req.Header = maps.Clone(r.headers)
	}

	res, resErr := r.httpClient.Do(req)

	if resErr != nil {
		return nil, false, nil, resErr
	}

	defer func() {
		_, _ = io.ReadAll(res.Body)
		_ = res.Body.Close()
	}()

	if err := checkForHTTPError(res.StatusCode, url); err != nil {
		return nil, false, res.Header, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := io.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, res.Header, ioErr // COVERAGE: there is no way to simulate this condition in unit tests
	}
	return body, cached, res.Header, nil
}
