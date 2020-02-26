package ldcomponents

import (
	"encoding/json"
	"fmt"
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

type requestor struct {
	sdkKey     string
	httpClient *http.Client
	baseURI    string
	headers    http.Header
	loggers    ldlog.Loggers
}

func newRequestor(context interfaces.ClientContext, httpClient *http.Client, baseURI string) *requestor {
	var decoratedClient http.Client
	if httpClient != nil {
		decoratedClient = *httpClient
	} else {
		decoratedClient = *context.CreateHTTPClient()
	}
	decoratedClient.Transport = &httpcache.Transport{
		Cache:               httpcache.NewMemoryCache(),
		MarkCachedResponses: true,
		Transport:           decoratedClient.Transport,
	}

	httpRequestor := requestor{
		sdkKey:     context.GetSDKKey(),
		httpClient: &decoratedClient,
		baseURI:    baseURI,
		loggers:    context.GetLoggers(),
	}

	return &httpRequestor
}

func (r *requestor) requestAll() (allData, bool, error) {
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
		return allData{}, false, jsonErr
	}
	return data, cached, nil
}

func (r *requestor) requestResource(kind interfaces.VersionedDataKind, key string) (interfaces.VersionedData, error) {
	var resource string
	switch kind.GetNamespace() {
	case "segments":
		resource = LatestSegmentsPath + "/" + key
	case "features":
		resource = LatestFlagsPath + "/" + key
	default:
		return nil, fmt.Errorf("unexpected item type: %s", kind)
	}
	body, _, err := r.makeRequest(resource)
	if err != nil {
		return nil, err
	}
	item := kind.GetDefaultItem().(interfaces.VersionedData)
	err = json.Unmarshal(body, item)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *requestor) makeRequest(resource string) ([]byte, bool, error) {
	r.loggers.Debug("Polling LaunchDarkly for feature flag updates")
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

	if err := checkForHttpError(res.StatusCode, url); err != nil {
		return nil, false, err
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, ioErr := ioutil.ReadAll(res.Body)

	if ioErr != nil {
		return nil, false, ioErr
	}
	return body, cached, nil
}
