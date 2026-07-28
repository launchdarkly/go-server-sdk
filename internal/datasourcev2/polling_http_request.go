package datasourcev2

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"

	"github.com/gregjones/httpcache"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// pollingRequester is the internal implementation of getting flag/segment data from the LD polling endpoints.
type pollingRequester struct {
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

func newPollingRequester(
	context subsystems.ClientContext,
	httpClient *http.Client,
	baseURI string,
	filterKey string,
) *pollingRequester {
	if httpClient == nil {
		httpClient = context.GetHTTP().CreateHTTPClient()
	}

	modifiedClient := *httpClient
	modifiedClient.Transport = &httpcache.Transport{
		Cache:               httpcache.NewMemoryCache(),
		MarkCachedResponses: true,
		Transport:           httpClient.Transport,
	}

	return &pollingRequester{
		httpClient: &modifiedClient,
		baseURI:    baseURI,
		filterKey:  filterKey,
		headers:    context.GetHTTP().DefaultHeaders,
		loggers:    context.GetLogging().Loggers,
	}
}

func (r *pollingRequester) BaseURI() string {
	return r.baseURI
}

func (r *pollingRequester) FilterKey() string {
	return r.filterKey
}

func (r *pollingRequester) Request(
	ctx context.Context,
	selector subsystems.Selector,
) (*subsystems.ChangeSet, http.Header, error) {
	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	body, cached, headers, err := r.makeRequest(ctx, endpoints.PollingRequestV2Path, selector)
	if err != nil {
		return nil, headers, err
	}
	if cached {
		return subsystems.NewChangeSetBuilder().NoChanges(), headers, nil
	}

	changeSet, err := parsePollingPayload(ctx, body)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, headers, err
		}
		return nil, headers, malformedJSONError{err}
	}
	return changeSet, headers, nil
}

func (r *pollingRequester) makeRequest(
	ctx context.Context,
	resource string,
	selector subsystems.Selector,
) ([]byte, bool, http.Header, error) {
	req, reqErr := http.NewRequestWithContext(ctx, "GET", endpoints.AddPath(r.baseURI, resource), nil)
	if reqErr != nil {
		reqErr = fmt.Errorf(
			"unable to create a poll request; this is not a network problem, most likely a bad base URI: %w",
			reqErr,
		)
		return nil, false, nil, reqErr
	}

	params := url.Values{}

	if selector.IsDefined() {
		params["basis"] = []string{selector.State()}
	}

	if r.filterKey != "" {
		params["filter"] = []string{r.filterKey}
	}

	if len(params) > 0 {
		req.URL.RawQuery = params.Encode()
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

	if err := checkForHTTPError(res.StatusCode, res.Header, url); err != nil {
		return nil, false, res.Header, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := io.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, res.Header, ioErr // COVERAGE: there is no way to simulate this condition in unit tests
	}
	return body, cached, res.Header, nil
}
