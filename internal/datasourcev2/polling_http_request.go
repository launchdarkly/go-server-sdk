package datasourcev2

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/gregjones/httpcache"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"golang.org/x/exp/maps"
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

func (r *pollingRequester) Request() (*fdv2proto.ChangeSet, error) {
	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	body, cached, err := r.makeRequest(endpoints.PollingRequestPath)
	if err != nil {
		return nil, err
	}
	if cached {
		return fdv2proto.NewChangeSetBuilder().NoChanges(), nil
	}

	var payload fdv2proto.PollingPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, malformedJSONError{err}
	}

	changeSet := fdv2proto.NewChangeSetBuilder()

	for _, event := range payload.Events {
		switch event.Name {
		case fdv2proto.EventServerIntent:
			var serverIntent fdv2proto.ServerIntent
			err := json.Unmarshal(event.Data, &serverIntent)
			if err != nil {
				return nil, err
			}
			if err := changeSet.Start(serverIntent); err != nil {
				return nil, err
			}
		case fdv2proto.EventPutObject:
			var put fdv2proto.PutObject
			if err := json.Unmarshal(event.Data, &put); err != nil {
				return nil, err
			}
			changeSet.AddPut(put.Kind, put.Key, put.Version, put.Object)
		case fdv2proto.EventDeleteObject:
			var deleteObject fdv2proto.DeleteObject
			if err := json.Unmarshal(event.Data, &deleteObject); err != nil {
				return nil, err
			}
			changeSet.AddDelete(deleteObject.Kind, deleteObject.Key, deleteObject.Version)
		case fdv2proto.EventPayloadTransferred:
			var selector fdv2proto.Selector
			if err := json.Unmarshal(event.Data, &selector); err != nil {
				return nil, err
			}
			return changeSet.Finish(selector)
		}
	}

	return nil, fmt.Errorf("malformed protocol")
}

func (r *pollingRequester) makeRequest(resource string) ([]byte, bool, error) {
	req, reqErr := http.NewRequest("GET", endpoints.AddPath(r.baseURI, resource), nil)
	if reqErr != nil {
		reqErr = fmt.Errorf(
			"unable to create a poll request; this is not a network problem, most likely a bad base URI: %w",
			reqErr,
		)
		return nil, false, reqErr
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
		return nil, false, resErr
	}

	defer func() {
		_, _ = io.ReadAll(res.Body)
		_ = res.Body.Close()
	}()

	if err := checkForHTTPError(res.StatusCode, url); err != nil {
		return nil, false, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := io.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, ioErr // COVERAGE: there is no way to simulate this condition in unit tests
	}
	return body, cached, nil
}
