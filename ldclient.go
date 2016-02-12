package ldclient

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"
)

const Version string = "0.0.3"

// The LaunchDarkly client. Client instances are thread-safe.
// Applications should instantiate a single instance for the lifetime
// of their application.
type LDClient struct {
	apiKey          string
	config          Config
	eventProcessor  *eventProcessor
	offline         bool
	streamProcessor *streamProcessor
	requestor       *requestor
}

// Exposes advanced configuration options for the LaunchDarkly client.
type Config struct {
	BaseUri       string
	StreamUri     string
	EventsUri     string
	Capacity      int
	FlushInterval time.Duration
	Logger        *log.Logger
	Timeout       time.Duration
	Stream        bool
	FeatureStore  FeatureStore
	UseLdd        bool
	SendEvents    bool
}

// Provides the default configuration options for the LaunchDarkly client.
// The easiest way to create a custom configuration is to start with the
// default config, and set the custom options from there. For example:
//   var config = DefaultConfig
//   config.Capacity = 2000
var DefaultConfig = Config{
	BaseUri:       "https://app.launchdarkly.com",
	StreamUri:     "https://stream.launchdarkly.com",
	EventsUri:     "https://events.launchdarkly.com",
	Capacity:      1000,
	FlushInterval: 5 * time.Second,
	Logger:        log.New(os.Stderr, "[LaunchDarkly]", log.LstdFlags),
	Timeout:       3000 * time.Millisecond,
	Stream:        true,
	FeatureStore:  nil,
	UseLdd:        false,
	SendEvents:    true,
}

// Creates a new client instance that connects to LaunchDarkly with the default configuration. In most
// cases, you should use this method to instantiate your client.
func MakeClient(apiKey string) *LDClient {
	res := MakeCustomClient(apiKey, DefaultConfig)
	return &res
}

// Creates a new client instance that connects to LaunchDarkly with a custom configuration.
func MakeCustomClient(apiKey string, config Config) LDClient {
	var streamProcessor *streamProcessor

	config.BaseUri = strings.TrimRight(config.BaseUri, "/")

	requestor := newRequestor(apiKey, config)

	if config.Stream {
		streamProcessor = newStream(apiKey, config, requestor)
	}

	return LDClient{
		apiKey:          apiKey,
		config:          config,
		requestor:       requestor,
		eventProcessor:  newEventProcessor(apiKey, config),
		offline:         false,
		streamProcessor: streamProcessor,
	}
}

func (client *LDClient) Identify(user User) error {
	if client.offline {
		return nil
	}
	evt := NewIdentifyEvent(user)
	return client.eventProcessor.sendEvent(evt)
}

// Tracks that a user has performed an event. Custom data can be attached to the
// event, and is serialized to JSON using the encoding/json package (http://golang.org/pkg/encoding/json/).
func (client *LDClient) Track(key string, user User, data interface{}) error {
	if client.offline {
		return nil
	}
	evt := NewCustomEvent(key, user, data)
	return client.eventProcessor.sendEvent(evt)
}

// Puts the LaunchDarkly client in offline mode. In offline mode, no network calls will be made,
// and no events will be recorded. In addition, all calls to Toggle will return the default value.
func (client *LDClient) SetOffline() {
	client.offline = true
}

// Puts the LaunchDarkly client in online mode.
func (client *LDClient) SetOnline() {
	client.offline = false
}

// Returns whether the LaunchDarkly client is in offline mode.
func (client *LDClient) IsOffline() bool {
	return client.offline
}

// Eagerly initializes the stream connection. If InitializeStream is not called, the stream will
// be initialized lazily with the first call to Toggle.
func (client *LDClient) InitializeStream() {
	if client.config.Stream {
		client.streamProcessor.StartOnce()
	}
}

// Returns false if the LaunchDarkly client does not have an active connection to
// the LaunchDarkly streaming endpoint. If streaming mode is disabled in the client
// configuration, this will always return false.
func (client *LDClient) IsStreamDisconnected() bool {
	return client.config.Stream == false || client.streamProcessor == nil || client.streamProcessor.ShouldFallbackUpdate()
}

// Returns whether the LaunchDarkly client has received an initial response from
// the LaunchDarkly streaming endpoint. If this is the case, the client can service
// Toggle calls from the stream. If streaming mode is disabled in the client
// configuration, this will always return false.
func (client *LDClient) IsStreamInitialized() bool {
	return client.config.Stream && client.streamProcessor != nil && client.streamProcessor.Initialized()
}

// Stops the LaunchDarkly client from sending any additional events.
func (client *LDClient) Close() {
	client.eventProcessor.close()
}

// Immediately flushes queued events.
func (client *LDClient) Flush() {
	client.eventProcessor.flush()
}

// Returns the value of a boolean feature flag for a given user. Returns defaultVal if
// there is an error, if the flag doesn't exist, or the feature is turned off.
func (client *LDClient) Toggle(key string, user User, defaultVal bool) (bool, error) {
	value, err := client.evaluate(key, user, defaultVal)

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, err
	}

	result, ok := value.(bool)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-bool value")
	}

	client.sendFlagRequestEvent(key, user, value, defaultVal)
	return result, nil
}

// Returns the value of a feature flag (whose variations are integers) for the given user.
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off.
func (client *LDClient) IntVariation(key string, user User, defaultVal int) (int, error) {
	value, err := client.evaluate(key, user, float64(defaultVal))

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, err
	}

	// json numbers are deserialized into float64s
	result, ok := value.(float64)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-numeric value")
	}

	client.sendFlagRequestEvent(key, user, value, defaultVal)
	return int(result), nil
}

// Returns the value of a feature flag (whose variations are floats) for the given user.
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off.
func (client *LDClient) Float64Variation(key string, user User, defaultVal float64) (float64, error) {
	value, err := client.evaluate(key, user, defaultVal)

	if err != nil {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, err
	}

	result, ok := value.(float64)

	if !ok {
		client.sendFlagRequestEvent(key, user, defaultVal, defaultVal)
		return defaultVal, errors.New("Feature flag returned non-numeric value")
	}

	client.sendFlagRequestEvent(key, user, value, defaultVal)
	return result, nil
}

func (client *LDClient) sendFlagRequestEvent(key string, user User, value, defaultVal interface{}) error {
	if client.offline {
		return nil
	}
	evt := NewFeatureRequestEvent(key, user, value, defaultVal)
	return client.eventProcessor.sendEvent(evt)
}

func (client *LDClient) evaluate(key string, user User, defaultVal interface{}) (interface{}, error) {
	var feature Feature
	var streamErr error

	if client.IsOffline() {
		return defaultVal, nil
	}

	client.InitializeStream()
	if client.IsStreamInitialized() {
		var featurePtr *Feature
		featurePtr, streamErr = client.streamProcessor.GetFeature(key)

		if !client.config.UseLdd && client.IsStreamDisconnected() {
			go func() {
				if feature, err := client.requestor.makeRequest(key, true); err != nil {
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
		// If streaming mode is enabled, get the latest version of the feature
		// Otherwise, respect the TTL
		if featurePtr, reqErr := client.requestor.makeRequest(key, client.config.Stream); reqErr != nil {
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
