package ldclient

import (
	"encoding/json"
	"errors"
	"github.com/facebookgo/httpcontrol"
	"github.com/gregjones/httpcache"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

var Version string = "0.0.3"

type LDClient struct {
	apiKey          string
	config          Config
	httpClient      *http.Client
	eventProcessor  *eventProcessor
	offline         bool
	streamProcessor *StreamProcessor
}

type Config struct {
	BaseUri       string
	StreamUri     string
	Capacity      int
	FlushInterval time.Duration
	Logger        *log.Logger
	Timeout       time.Duration
	Stream        bool
	FeatureStore  FeatureStore
	UseLdd        bool
}

var DefaultConfig = Config{
	BaseUri:       "https://app.launchdarkly.com",
	StreamUri:     "https://stream.launchdarkly.com",
	Capacity:      1000,
	FlushInterval: 5 * time.Second,
	Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
	Timeout:       3000 * time.Millisecond,
	Stream:        false,
	FeatureStore:  nil,
	UseLdd:        false,
}

func MakeCustomClient(apiKey string, config Config) LDClient {
	var streamProcessor *StreamProcessor

	config.BaseUri = strings.TrimRight(config.BaseUri, "/")

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

	if config.Stream {
		streamProcessor = newStream(apiKey, config)
	}

	return LDClient{
		apiKey:          apiKey,
		config:          config,
		httpClient:      httpClient,
		eventProcessor:  newEventProcessor(apiKey, config),
		offline:         false,
		streamProcessor: streamProcessor,
	}
}

func MakeClient(apiKey string) *LDClient {
	res := MakeCustomClient(apiKey, DefaultConfig)
	return &res
}

func (client *LDClient) Identify(user User) error {
	if client.offline {
		return nil
	}
	evt := NewIdentifyEvent(user)
	return client.eventProcessor.sendEvent(evt)
}

func (client *LDClient) Track(key string, user User, data interface{}) error {
	if client.offline {
		return nil
	}
	evt := NewCustomEvent(key, user, data)
	return client.eventProcessor.sendEvent(evt)
}

func (client *LDClient) sendFlagRequestEvent(key string, user User, value interface{}) error {
	if client.offline {
		return nil
	}
	evt := NewFeatureRequestEvent(key, user, value)
	return client.eventProcessor.sendEvent(evt)
}

func (client *LDClient) SetOffline() {
	client.offline = true
}

func (client *LDClient) SetOnline() {
	client.offline = false
}

func (client *LDClient) IsOffline() bool {
	return client.offline
}

func (client *LDClient) IsStreamDisconnected() bool {
	return client.config.Stream == false || client.streamProcessor == nil || client.streamProcessor.ShouldFallbackUpdate()
}

func (client *LDClient) IsStreamInitialized() bool {
	return client.config.Stream && client.streamProcessor != nil && client.streamProcessor.Initialized()
}

func (client *LDClient) Close() {
	client.eventProcessor.close()
}

func (client *LDClient) Flush() {
	client.eventProcessor.flush()
}

func (client *LDClient) GetFlag(key string, user User, defaultVal bool) (bool, error) {
	return client.Toggle(key, user, defaultVal)
}

func (client *LDClient) makeRequest(key string) (*Feature, error) {
	var feature Feature

	req, reqErr := http.NewRequest("GET", client.config.BaseUri+"/api/eval/features/"+key, nil)

	if reqErr != nil {
		return nil, reqErr
	}

	req.Header.Add("Authorization", "api_key "+client.apiKey)
	req.Header.Add("User-Agent", "GoClient/"+Version)

	res, resErr := client.httpClient.Do(req)

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
	} else {
		return &feature, nil
	}

}

func (client *LDClient) evaluate(key string, user User, defaultVal interface{}) (interface{}, error) {
	var feature Feature
	var streamErr error

	if client.IsOffline() {
		return defaultVal, nil
	}

	if client.IsStreamInitialized() {
		var featurePtr *Feature
		featurePtr, streamErr = client.streamProcessor.GetFeature(key)

		if !client.config.UseLdd && client.IsStreamDisconnected() {
			go func() {
				if feature, err := client.makeRequest(key); err != nil {
					client.config.Logger.Printf("Failed to update feature in fallback mode. Flag values may be stale.")
				} else {
					client.streamProcessor.store.Upsert(*feature.Key, *feature)
				}
			}()
		}

		if streamErr != nil {
			client.config.Logger.Printf("Encountered error in stream: %+v", streamErr)
			return defaultVal, streamErr
		}

		if featurePtr != nil {
			feature = *featurePtr
		} else {
			return defaultVal, errors.New("Unknown feature key. Verify that this feature key exists. Returning default value.")
		}
	} else {
		if featurePtr, reqErr := client.makeRequest(key); reqErr != nil {
			return defaultVal, reqErr
		} else {
			feature = *featurePtr
		}
	}

	value, pass := feature.Evaluate(user)

	if pass {
		return defaultVal, nil
	}

	return value, nil
}

func (client *LDClient) Toggle(key string, user User, defaultVal bool) (bool, error) {
	value, err := client.evaluate(key, user, defaultVal)

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, err
	}

	result, ok := value.(bool)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-bool value")
	}

	client.sendFlagRequestEvent(key, user, value)
	return result, nil
}

func (client *LDClient) IntVariation(key string, user User, defaultVal int) (int, error) {
	value, err := client.evaluate(key, user, float64(defaultVal))

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, err
	}

	// json numbers are deserialized into float64s
	result, ok := value.(float64)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-numeric value")
	}

	client.sendFlagRequestEvent(key, user, value)
	return int(result), nil
}

func (client *LDClient) Float64Variation(key string, user User, defaultVal float64) (float64, error) {
	value, err := client.evaluate(key, user, defaultVal)

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, err
	}

	result, ok := value.(float64)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-numeric value")
	}

	client.sendFlagRequestEvent(key, user, value)
	return result, nil
}
