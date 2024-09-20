package datasourcev2

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/internal/endpoints"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/gregjones/httpcache"
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
func (r *pollingRequester) Request() (*PollingResponse, error) {

	if r.loggers.IsDebugEnabled() {
		r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
	}

	body, cached, err := r.makeRequest(endpoints.PollingRequestPath)
	if err != nil {
		return nil, err
	}
	if cached {
		return NewCachedPollingResponse(), nil
	}

	var payload pollingPayload
	if err = json.Unmarshal(body, &payload); err != nil {
		return nil, malformedJSONError{err}
	}

	parseItem := func(r jreader.Reader, kind datakinds.DataKindInternal) (ldstoretypes.ItemDescriptor, error) {
		item, err := kind.DeserializeFromJSONReader(&r)
		return item, err
	}

	updates := make([]fdv2proto.Event, 0, len(payload.Events))

	var intent fdv2proto.IntentCode

	for _, event := range payload.Events {
		switch event.Event() {
		case fdv2proto.EventServerIntent:
			{
				var serverIntent serverIntent
				err := json.Unmarshal([]byte(event.Data()), &serverIntent)
				if err != nil {
					return nil, err
				} else if len(serverIntent.Payloads) == 0 {
					return nil, errors.New("server-intent event has no payloads")
				}

				intent = serverIntent.Payloads[0].Code
				if intent == "none" {
					return NewCachedPollingResponse(), nil
				}
			}
		case fdv2proto.EventPutObject:
			{
				r := jreader.NewReader([]byte(event.Data()))
				var kind, key string
				var item ldstoretypes.ItemDescriptor
				var err error
				var dataKind datakinds.DataKindInternal

				for obj := r.Object().WithRequiredProperties([]string{versionField, kindField, "key", "object"}); obj.Next(); {
					switch string(obj.Name()) {
					case versionField:
						// version = r.Int()
					case kindField:
						kind = strings.TrimRight(r.String(), "s")
						dataKind = dataKindFromKind(kind)
					case "key":
						key = r.String()
					case "object":
						item, err = parseItem(r, dataKind)
						if err != nil {
							return nil, err
						}
					}
				}
				updates = append(updates, fdv2proto.PutObject{Kind: dataKind, Key: key, Object: item})
			}
		case fdv2proto.EventDeleteObject:
			{
				r := jreader.NewReader([]byte(event.Data()))
				var version int
				var dataKind datakinds.DataKindInternal
				var kind, key string

				for obj := r.Object().WithRequiredProperties([]string{versionField, kindField, keyField}); obj.Next(); {
					switch string(obj.Name()) {
					case versionField:
						version = r.Int()
					case kindField:
						kind = strings.TrimRight(r.String(), "s")
						dataKind = dataKindFromKind(kind)
						if dataKind == nil {
							//nolint: godox
							// TODO: We are skipping here without showing a warning. Need to address that later.
							continue
						}
					case keyField:
						key = r.String()
					}
				}
				updates = append(updates, fdv2proto.DeleteObject{Kind: dataKind, Key: key, Version: version})

			}
		case fdv2proto.EventPayloadTransferred:
			// TODO: deserialize the state and create a fdv2proto.Selector.
		}
	}

	if intent == "" {
		return nil, errors.New("no server-intent event found in polling response")
	}

	return NewPollingResponse(intent, updates, fdv2proto.NoSelector()), nil
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
