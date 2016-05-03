package ldclient

import (
	"encoding/json"
	"fmt"
	"github.com/facebookgo/httpcontrol"
	"github.com/gregjones/httpcache"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

const (
	LatestFlagsPath = "/sdk/latest-flags/"
	FlagsPath       = "/sdk/flags/"
)

type requestor struct {
	apiKey     string
	httpClient *http.Client
	config     Config
}

func newRequestor(apiKey string, config Config) *requestor {
	baseTransport := httpcontrol.Transport{
		RequestTimeout: config.Timeout,
		DialTimeout:    config.Timeout,
		DialKeepAlive:  1 * time.Minute,
		MaxTries:       3,
	}

	cachingTransport := &httpcache.Transport{
		Cache:               httpcache.NewMemoryCache(),
		MarkCachedResponses: true,
		Transport:           &baseTransport,
	}

	httpClient := cachingTransport.Client()

	requestor := requestor{
		apiKey:     apiKey,
		httpClient: httpClient,
		config:     config,
	}

	return &requestor
}

func (r *requestor) requestAllFlags(latest bool) (map[string]*FeatureFlag, bool, error) {
	var features map[string]*FeatureFlag
	body, cached, err := r.makeRequest(r.makeFlagsResource(latest))
	if err != nil {
		return nil, false, err
	}
	if cached {
		return nil, true, nil
	}
	jsonErr := json.Unmarshal(body, &features)

	if jsonErr != nil {
		return nil, false, jsonErr
	}
	return features, cached, nil
}

func (r *requestor) requestFlag(key string, latest bool) (*FeatureFlag, error) {
	var feature FeatureFlag
	resource := r.makeFlagsResource(latest) + key
	body, _, err := r.makeRequest(resource)
	if err != nil {
		return nil, err
	}

	jsonErr := json.Unmarshal(body, &feature)

	if jsonErr != nil {
		return nil, jsonErr
	}
	return &feature, nil
}

func (r *requestor) makeFlagsResource(latest bool) string {
	if latest {
		return LatestFlagsPath
	} else {
		return FlagsPath
	}
}

func (r *requestor) makeRequest(resource string) ([]byte, bool, error) {
	req, reqErr := http.NewRequest("GET", r.config.BaseUri+resource, nil)
	url := req.URL.RequestURI()
	if reqErr != nil {
		return nil, false, reqErr
	}

	req.Header.Add("Authorization", "api_key "+r.apiKey)
	req.Header.Add("User-Agent", "GoClient/"+Version)

	res, resErr := r.httpClient.Do(req)

	defer func() {
		if res != nil && res.Body != nil {
			ioutil.ReadAll(res.Body)
			res.Body.Close()
		}
	}()

	if resErr != nil {
		return nil, false, resErr
	}

	if res.StatusCode == http.StatusUnauthorized {
		return nil, false, fmt.Errorf("Invalid API key when accessing URL: %s. Verify that your API key is correct.", url)
	}

	if res.StatusCode == http.StatusNotFound {
		return nil, false, fmt.Errorf("Resource not found when accessing URL: %s. Verify that this resource exists.", url)
	}

	if res.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("Unexpected response code: %s when accessing URL: %s"+strconv.Itoa(res.StatusCode), url)
	}

	cached := res.Header.Get(httpcache.XFromCache) != ""

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return nil, false, err
	}
	return body, cached, err
}
