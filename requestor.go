package ldclient

import (
	"encoding/json"
	"errors"
	"github.com/facebookgo/httpcontrol"
	"github.com/gregjones/httpcache"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
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
	}

	return &requestor
}

func (r *requestor) makeRequest(key string, latest bool) (*Feature, error) {
	var feature Feature

	var resource string

	if latest {
		resource = "/api/eval/latest-features/"
	} else {
		resource = "/api/eval/features/"
	}

	req, reqErr := http.NewRequest("GET", r.config.BaseUri+resource+key, nil)

	if reqErr != nil {
		return nil, reqErr
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
		return nil, resErr
	}

	if res.StatusCode == http.StatusUnauthorized {
		return nil, errors.New("Invalid API key. Verify that your API key is correct. Returning default value.")
	}

	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("Unknown feature key. Verify that this feature key exists. Returning default value.")
	}

	if res.StatusCode != http.StatusOK {
		return nil, errors.New("Unexpected response code: " + strconv.Itoa(res.StatusCode))
	}

	body, err := ioutil.ReadAll(res.Body)

	if err != nil {
		return nil, err
	}

	jsonErr := json.Unmarshal(body, &feature)

	if jsonErr != nil {
		return nil, jsonErr
	}
	return &feature, nil
}
