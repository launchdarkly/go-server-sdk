package datasourcev2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"maps"

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

func (r *pollingRequester) Request(ctx context.Context, selector subsystems.Selector) (*subsystems.ChangeSet, error) {
	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	body, cached, err := r.makeRequest(ctx, endpoints.PollingRequestV2Path, selector)
	if err != nil {
		return nil, err
	}
	if cached {
		return subsystems.NewChangeSetBuilder().NoChanges(), nil
	}

	var payload subsystems.PollingPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, malformedJSONError{err}
	}

	changeSet := subsystems.NewChangeSetBuilder()

	for _, event := range payload.Events {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			switch event.Name {
			case subsystems.EventServerIntent:
				var serverIntent subsystems.ServerIntent
				err := json.Unmarshal(event.Data, &serverIntent)
				if err != nil {
					return nil, err
				}
				if serverIntent.Payload.Code == subsystems.IntentNone {
					return changeSet.NoChanges(), nil
				}
				if err := changeSet.Start(serverIntent); err != nil {
					return nil, err
				}
			case subsystems.EventPutObject:
				var put subsystems.PutObject
				if err := json.Unmarshal(event.Data, &put); err != nil {
					return nil, err
				}
				changeSet.AddPut(put.Kind, put.Key, put.Version, put.Object)
			case subsystems.EventDeleteObject:
				var deleteObject subsystems.DeleteObject
				if err := json.Unmarshal(event.Data, &deleteObject); err != nil {
					return nil, err
				}
				changeSet.AddDelete(deleteObject.Kind, deleteObject.Key, deleteObject.Version)
			case subsystems.EventPayloadTransferred:
				var selector subsystems.Selector
				if err := json.Unmarshal(event.Data, &selector); err != nil {
					return nil, err
				}
				return changeSet.Finish(selector)
			}
		}
	}

	return nil, fmt.Errorf("didn't receive any known protocol events in polling payload")
}

func (r *pollingRequester) makeRequest(
	ctx context.Context,
	resource string,
	selector subsystems.Selector,
) ([]byte, bool, error) {
	req, reqErr := http.NewRequestWithContext(ctx, "GET", endpoints.AddPath(r.baseURI, resource), nil)
	if reqErr != nil {
		reqErr = fmt.Errorf(
			"unable to create a poll request; this is not a network problem, most likely a bad base URI: %w",
			reqErr,
		)
		return nil, false, reqErr
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
		return nil, false, resErr
	}

	defer func() {
		_, _ = io.ReadAll(res.Body)
		_ = res.Body.Close()
	}()

	if err := checkForHTTPError(res.StatusCode, res.Header, url); err != nil {
		return nil, false, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := io.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, ioErr // COVERAGE: there is no way to simulate this condition in unit tests
	}
	return body, cached, nil
}
