// Package ldclient is the main package for the LaunchDarkly SDK.
//
// This package contains the types and methods that most applications will use. The most commonly
// used other packages are "ldlog" (the SDK's logging abstraction) and database integrations such
// as "redis" and "lddynamodb".
package ldclient

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldevents"
)

// Version is the client version.
const Version = "5.0.0"

// LDClient is the LaunchDarkly client. Client instances are thread-safe.
// Applications should instantiate a single instance for the lifetime
// of their application.
type LDClient struct {
	sdkKey         string
	config         Config
	eventProcessor ldevents.EventProcessor
	dataSource     interfaces.DataSource
	store          interfaces.DataStore
	evaluator      ldeval.Evaluator
}

// Logger is a generic logger interface.
type Logger interface {
	Println(...interface{})
	Printf(string, ...interface{})
}

type nullDataSource struct{}

func (n nullDataSource) Initialized() bool {
	return true
}

func (n nullDataSource) Close() error {
	return nil
}

func (n nullDataSource) Start(closeWhenReady chan<- struct{}) {
	close(closeWhenReady)
}

// Implementation of ldeval.DataProvider
type clientEvaluatorDataProvider struct {
	store interfaces.DataStore
}

func (c *clientEvaluatorDataProvider) GetFeatureFlag(key string) (ldeval.FeatureFlag, bool) {
	data, err := c.store.Get(Features, key)
	if data != nil && err == nil {
		if flag, ok := data.(*ldeval.FeatureFlag); ok {
			return *flag, true
		}
	}
	return ldeval.FeatureFlag{}, false
}

func (c *clientEvaluatorDataProvider) GetSegment(key string) (ldeval.Segment, bool) {
	data, err := c.store.Get(Segments, key)
	if data != nil && err == nil {
		if segment, ok := data.(*ldeval.Segment); ok {
			return *segment, true
		}
	}
	return ldeval.Segment{}, false
}

// Implementation of ldeval.PrerequisiteFlagEventRecorder
type clientEvaluatorEventSink struct {
	user                *lduser.User
	events              []ldevents.FeatureRequestEvent
	sendReasonsInEvents bool
}

func (c *clientEvaluatorEventSink) recordPrerequisiteEvent(params ldeval.PrerequisiteFlagEvent) {
	event := ldevents.NewSuccessfulEvalEvent(&params.PrerequisiteFlag, *c.user, params.PrerequisiteResult.VariationIndex,
		params.PrerequisiteResult.Value, ldvalue.Null(), params.PrerequisiteResult.Reason, c.sendReasonsInEvents,
		&params.TargetFlagKey)
	c.events = append(c.events, event)
}

// Initialization errors
var (
	ErrInitializationTimeout = errors.New("timeout encountered waiting for LaunchDarkly client initialization")
	ErrInitializationFailed  = errors.New("LaunchDarkly client initialization failed")
	ErrClientNotInitialized  = errors.New("feature flag evaluation called before LaunchDarkly client initialization completed")
)

// MakeClient creates a new client instance that connects to LaunchDarkly with the default configuration. In most
// cases, you should use this method to instantiate your client. The optional duration parameter allows callers to
// block until the client has connected to LaunchDarkly and is properly initialized.
func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	return MakeCustomClient(sdkKey, DefaultConfig, waitFor)
}

// MakeCustomClient creates a new client instance that connects to LaunchDarkly with a custom configuration. The optional duration parameter allows callers to
// block until the client has connected to LaunchDarkly and is properly initialized.
func MakeCustomClient(sdkKey string, config Config, waitFor time.Duration) (*LDClient, error) {
	closeWhenReady := make(chan struct{})

	config.BaseUri = strings.TrimRight(config.BaseUri, "/")
	config.EventsUri = strings.TrimRight(config.EventsUri, "/")
	if config.PollInterval < MinimumPollInterval {
		config.PollInterval = MinimumPollInterval
	}
	config.UserAgent = strings.TrimSpace("GoClient/" + Version + " " + config.UserAgent)

	config.Loggers.Init()
	config.Loggers.Infof("Starting LaunchDarkly client %s", Version)

	if config.DataStore == nil {
		factory := config.DataStoreFactory
		if factory == nil {
			factory = NewInMemoryDataStoreFactory()
		}
		store, err := factory(config)
		if err != nil {
			return nil, err
		}
		config.DataStore = store
	}

	evaluator := ldeval.NewEvaluator(&clientEvaluatorDataProvider{config.DataStore})

	defaultHTTPClient := config.newHTTPClient()

	client := LDClient{
		sdkKey:    sdkKey,
		config:    config,
		store:     config.DataStore,
		evaluator: evaluator,
	}

	if !config.DiagnosticOptOut && config.SendEvents && !config.Offline {
		config.diagnosticsManager = createDiagnosticsManager(sdkKey, config, waitFor)
	}

	if config.EventProcessor != nil {
		client.eventProcessor = config.EventProcessor
	} else if config.SendEvents && !config.Offline {
		client.eventProcessor = createDefaultEventProcessor(sdkKey, config, defaultHTTPClient, config.diagnosticsManager)
	} else {
		client.eventProcessor = ldevents.NewNullEventProcessor()
	}

	factory := config.DataSourceFactory
	if factory == nil {
		factory = createDefaultDataSource(defaultHTTPClient)
	}
	var err error
	client.dataSource, err = factory(sdkKey, config)
	if err != nil {
		return nil, err
	}
	client.dataSource.Start(closeWhenReady)
	if waitFor > 0 && !config.Offline && !config.UseLdd {
		config.Loggers.Infof("Waiting up to %d milliseconds for LaunchDarkly client to start...",
			waitFor/time.Millisecond)
	}
	timeout := time.After(waitFor)
	for {
		select {
		case <-closeWhenReady:
			if !client.dataSource.Initialized() {
				config.Loggers.Warn("LaunchDarkly client initialization failed")
				return &client, ErrInitializationFailed
			}

			config.Loggers.Info("Successfully initialized LaunchDarkly client!")
			return &client, nil
		case <-timeout:
			if waitFor > 0 {
				config.Loggers.Warn("Timeout encountered waiting for LaunchDarkly client initialization")
				return &client, ErrInitializationTimeout
			}

			go func() { <-closeWhenReady }() // Don't block the DataSource when not waiting
			return &client, nil
		}
	}
}

func createDefaultDataSource(httpClient *http.Client) func(string, Config) (interfaces.DataSource, error) {
	return func(sdkKey string, config Config) (interfaces.DataSource, error) {
		if config.Offline {
			config.Loggers.Info("Started LaunchDarkly client in offline mode")
			return nullDataSource{}, nil
		}
		if config.UseLdd {
			config.Loggers.Info("Started LaunchDarkly client in LDD mode")
			return nullDataSource{}, nil
		}
		requestor := newRequestor(sdkKey, config, httpClient)
		if config.Stream {
			return newStreamProcessor(sdkKey, config, requestor), nil
		}
		config.Loggers.Warn("You should only disable the streaming API if instructed to do so by LaunchDarkly support")
		return newPollingProcessor(config, requestor), nil
	}
}

func createDefaultEventProcessor(sdkKey string, config Config, client *http.Client, diagnosticsManager *ldevents.DiagnosticsManager) ldevents.EventProcessor {
	headers := make(http.Header)
	addBaseHeaders(headers, sdkKey, config)
	eventsConfig := ldevents.EventsConfiguration{
		AllAttributesPrivate:        config.AllAttributesPrivate,
		Capacity:                    config.Capacity,
		DiagnosticRecordingInterval: config.DiagnosticRecordingInterval,
		DiagnosticURI:               strings.TrimRight(config.EventsUri, "/") + "/diagnostic",
		DiagnosticsManager:          diagnosticsManager,
		EventsURI:                   strings.TrimRight(config.EventsUri, "/") + "/bulk",
		FlushInterval:               config.FlushInterval,
		Headers:                     headers,
		HTTPClient:                  client,
		InlineUsersInEvents:         config.InlineUsersInEvents,
		Loggers:                     config.Loggers,
		LogUserKeyInErrors:          config.LogUserKeyInErrors,
		PrivateAttributeNames:       config.PrivateAttributeNames,
		UserKeysCapacity:            config.UserKeysCapacity,
		UserKeysFlushInterval:       config.UserKeysFlushInterval,
	}
	return ldevents.NewDefaultEventProcessor(eventsConfig)
}

// Identify reports details about a a user.
func (client *LDClient) Identify(user lduser.User) error {
	if user.GetKey() == "" {
		client.config.Loggers.Warn("Identify called with empty user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	evt := ldevents.NewIdentifyEvent(user)
	client.eventProcessor.SendEvent(evt)
	return nil
}

// TrackEvent reports that a user has performed an event.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard. If you want to associate additional data with this event, use TrackData
// or TrackMetric.
func (client *LDClient) TrackEvent(eventName string, user lduser.User) error {
	return client.TrackData(eventName, user, ldvalue.Null())
}

// TrackData reports that a user has performed an event, and associates it with custom data.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the ldvalue.Value type, that
// will be sent with the event. If no such value is needed, use ldvalue.Null() (or call TrackEvent
// instead). To send a numeric value for experimentation, use TrackMetric.
func (client *LDClient) TrackData(eventName string, user lduser.User, data ldvalue.Value) error {
	if user.GetKey() == "" {
		client.config.Loggers.Warn("Track called with empty/nil user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	client.eventProcessor.SendEvent(ldevents.NewCustomEvent(eventName, user, data, false, 0))
	return nil
}

// TrackMetric reports that a user has performed an event, and associates it with a numeric value.
// This value is used by the LaunchDarkly experimentation feature in numeric custom metrics, and will also
// be returned as part of the custom event for Data Export.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the ldvalue.Value type, that
// will be sent with the event. If no such value is needed, use ldvalue.Null().
func (client *LDClient) TrackMetric(eventName string, user lduser.User, metricValue float64, data ldvalue.Value) error {
	if user.GetKey() == "" {
		client.config.Loggers.Warn("Track called with empty/nil user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	client.eventProcessor.SendEvent(ldevents.NewCustomEvent(eventName, user, data, true, metricValue))
	return nil
}

// IsOffline returns whether the LaunchDarkly client is in offline mode.
func (client *LDClient) IsOffline() bool {
	return client.config.Offline
}

// SecureModeHash generates the secure mode hash value for a user
// See https://github.com/launchdarkly/js-client#secure-mode
func (client *LDClient) SecureModeHash(user lduser.User) string {
	key := []byte(client.sdkKey)
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(user.GetKey()))
	return hex.EncodeToString(h.Sum(nil))
}

// Initialized returns whether the LaunchDarkly client is initialized.
func (client *LDClient) Initialized() bool {
	return client.IsOffline() || client.config.UseLdd || client.dataSource.Initialized()
}

// Close shuts down the LaunchDarkly client. After calling this, the LaunchDarkly client
// should no longer be used. The method will block until all pending analytics events (if any)
// been sent.
func (client *LDClient) Close() error {
	client.config.Loggers.Info("Closing LaunchDarkly client")
	if client.IsOffline() {
		return nil
	}
	_ = client.eventProcessor.Close()
	_ = client.dataSource.Close()
	if c, ok := client.store.(io.Closer); ok { // not all DataStores implement Closer
		_ = c.Close()
	}
	return nil
}

// Flush tells the client that all pending analytics events (if any) should be delivered as soon
// as possible. Flushing is asynchronous, so this method will return before it is complete.
// However, if you call Close(), events are guaranteed to be sent before that method returns.
func (client *LDClient) Flush() {
	client.eventProcessor.Flush()
}

// AllFlagsState returns an object that encapsulates the state of all feature flags for a
// given user, including the flag values and also metadata that can be used on the front end.
// You may pass any combination of ClientSideOnly, WithReasons, and DetailsOnlyForTrackedFlags
// as optional parameters to control what data is included.
//
// The most common use case for this method is to bootstrap a set of client-side feature flags
// from a back-end service.
func (client *LDClient) AllFlagsState(user lduser.User, options ...FlagsStateOption) FeatureFlagsState {
	valid := true
	if client.IsOffline() {
		client.config.Loggers.Warn("Called AllFlagsState in offline mode. Returning empty state")
		valid = false
	} else if !client.Initialized() {
		if client.store.Initialized() {
			client.config.Loggers.Warn("Called AllFlagsState before client initialization; using last known values from data store")
		} else {
			client.config.Loggers.Warn("Called AllFlagsState before client initialization. Data store not available; returning empty state")
			valid = false
		}
	}

	if !valid {
		return FeatureFlagsState{valid: false}
	}

	items, err := client.store.All(Features)
	if err != nil {
		client.config.Loggers.Warn("Unable to fetch flags from data store. Returning empty state. Error: " + err.Error())
		return FeatureFlagsState{valid: false}
	}

	state := newFeatureFlagsState()
	clientSideOnly := hasFlagsStateOption(options, ClientSideOnly)
	withReasons := hasFlagsStateOption(options, WithReasons)
	detailsOnlyIfTracked := hasFlagsStateOption(options, DetailsOnlyForTrackedFlags)
	for _, item := range items {
		if flag, ok := item.(*ldeval.FeatureFlag); ok {
			if clientSideOnly && !flag.ClientSide {
				continue
			}
			result := client.evaluator.Evaluate(*flag, user, nil)
			var reason ldreason.EvaluationReason
			if withReasons {
				reason = result.Reason
			}
			state.addFlag(*flag, result.Value, result.VariationIndex, reason, detailsOnlyIfTracked)
		}
	}

	return state
}

// BoolVariation returns the value of a boolean feature flag for a given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
func (client *LDClient) BoolVariation(key string, user lduser.User, defaultVal bool) (bool, error) {
	detail, err := client.variation(key, user, ldvalue.Bool(defaultVal), true, false)
	return detail.Value.BoolValue(), err
}

// BoolVariationDetail is the same as BoolVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
func (client *LDClient) BoolVariationDetail(key string, user lduser.User, defaultVal bool) (bool, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Bool(defaultVal), true, true)
	return detail.Value.BoolValue(), detail, err
}

// IntVariation returns the value of a feature flag (whose variations are integers) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// If the flag variation has a numeric value that is not an integer, it is rounded toward zero (truncated).
func (client *LDClient) IntVariation(key string, user lduser.User, defaultVal int) (int, error) {
	detail, err := client.variation(key, user, ldvalue.Int(defaultVal), true, false)
	return detail.Value.IntValue(), err
}

// IntVariationDetail is the same as IntVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
func (client *LDClient) IntVariationDetail(key string, user lduser.User, defaultVal int) (int, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Int(defaultVal), true, true)
	return detail.Value.IntValue(), detail, err
}

// Float64Variation returns the value of a feature flag (whose variations are floats) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
func (client *LDClient) Float64Variation(key string, user lduser.User, defaultVal float64) (float64, error) {
	detail, err := client.variation(key, user, ldvalue.Float64(defaultVal), true, false)
	return detail.Value.Float64Value(), err
}

// Float64VariationDetail is the same as Float64Variation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
func (client *LDClient) Float64VariationDetail(key string, user lduser.User, defaultVal float64) (float64, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Float64(defaultVal), true, true)
	return detail.Value.Float64Value(), detail, err
}

// StringVariation returns the value of a feature flag (whose variations are strings) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and has
// no off variation.
func (client *LDClient) StringVariation(key string, user lduser.User, defaultVal string) (string, error) {
	detail, err := client.variation(key, user, ldvalue.String(defaultVal), true, false)
	return detail.Value.StringValue(), err
}

// StringVariationDetail is the same as StringVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
func (client *LDClient) StringVariationDetail(key string, user lduser.User, defaultVal string) (string, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.String(defaultVal), true, true)
	return detail.Value.StringValue(), detail, err
}

// JSONVariation returns the value of a feature flag for the given user, allowing the value to be
// of any JSON type.
//
// The value is returned as an ldvalue.Value, which can be inspected or converted to other types using
// Value methods such as GetType() and BoolValue(). The defaultVal parameter also uses this type. For
// instance, if the values for this flag are JSON arrays:
//
//     defaultValAsArray := ldvalue.BuildArray().
//         Add(ldvalue.String("defaultFirstItem")).
//         Add(ldvalue.String("defaultSecondItem")).
//         Build()
//     result, err := client.JSONVariation(flagKey, user, defaultValAsArray)
//     firstItemAsString := result.GetByIndex(0).StringValue() // "defaultFirstItem", etc.
//
// You can also use unparsed json.RawMessage values:
//
//     defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"things":[1,2,3]}`))
//     result, err := client.JSONVariation(flagKey, user, defaultValAsJSON
//     resultAsRawJSON := result.AsRaw()
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off.
func (client *LDClient) JSONVariation(key string, user lduser.User, defaultVal ldvalue.Value) (ldvalue.Value, error) {
	detail, err := client.variation(key, user, defaultVal, false, false)
	return detail.Value, err
}

// JSONVariationDetail is the same as JSONVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
func (client *LDClient) JSONVariationDetail(key string, user lduser.User, defaultVal ldvalue.Value) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, defaultVal, false, true)
	return detail.Value, detail, err
}

// Generic method for evaluating a feature flag for a given user.
func (client *LDClient) variation(
	key string,
	user lduser.User,
	defaultVal ldvalue.Value,
	checkType bool,
	sendReasonsInEvents bool,
) (ldreason.EvaluationDetail, error) {
	if client.IsOffline() {
		return newEvaluationError(defaultVal, ldreason.EvalErrorClientNotReady), nil
	}
	result, flag, err := client.evaluateInternal(key, user, defaultVal, sendReasonsInEvents)
	if err != nil {
		result.Value = defaultVal
		result.VariationIndex = -1
	} else {
		if checkType && defaultVal.Type() != ldvalue.NullType && result.Value.Type() != defaultVal.Type() {
			result = newEvaluationError(defaultVal, ldreason.EvalErrorWrongType)
		}
	}

	var evt ldevents.FeatureRequestEvent
	if flag == nil {
		evt = ldevents.NewUnknownFlagEvent(key, user, defaultVal, result.Reason, sendReasonsInEvents) //nolint
	} else {
		evt = ldevents.NewSuccessfulEvalEvent(flag, user, result.VariationIndex, result.Value, defaultVal,
			result.Reason, sendReasonsInEvents, nil)
	}
	client.eventProcessor.SendEvent(evt)

	return result, err
}

// Performs all the steps of evaluation except for sending the feature request event (the main one;
// events for prerequisites will be sent).
func (client *LDClient) evaluateInternal(
	key string,
	user lduser.User,
	defaultVal ldvalue.Value,
	sendReasonsInEvents bool,
) (ldreason.EvaluationDetail, *ldeval.FeatureFlag, error) {
	if user.GetKey() == "" {
		client.config.Loggers.Warnf("User.Key is blank when evaluating flag: %s. Flag evaluation will proceed, but the user will not be stored in LaunchDarkly.", key)
	}

	var feature *ldeval.FeatureFlag
	var storeErr error
	var ok bool

	evalErrorResult := func(errKind ldreason.EvalErrorKind, flag *ldeval.FeatureFlag, err error) (ldreason.EvaluationDetail, *ldeval.FeatureFlag, error) {
		detail := newEvaluationError(defaultVal, errKind)
		if client.config.LogEvaluationErrors {
			client.config.Loggers.Warn(err)
		}
		return detail, flag, err
	}

	if !client.Initialized() {
		if client.store.Initialized() {
			client.config.Loggers.Warn("Feature flag evaluation called before LaunchDarkly client initialization completed; using last known values from data store")
		} else {
			return evalErrorResult(ldreason.EvalErrorClientNotReady, nil, ErrClientNotInitialized)
		}
	}

	data, storeErr := client.store.Get(Features, key)

	if storeErr != nil {
		client.config.Loggers.Errorf("Encountered error fetching feature from store: %+v", storeErr)
		detail := newEvaluationError(defaultVal, ldreason.EvalErrorException)
		return detail, nil, storeErr
	}

	if data != nil {
		feature, ok = data.(*ldeval.FeatureFlag)
		if !ok {
			return evalErrorResult(ldreason.EvalErrorException, nil,
				fmt.Errorf("unexpected data type (%T) found in store for feature key: %s. Returning default value", data, key))
		}
	} else {
		return evalErrorResult(ldreason.EvalErrorFlagNotFound, nil,
			fmt.Errorf("unknown feature key: %s. Verify that this feature key exists. Returning default value", key))
	}

	eventSink := clientEvaluatorEventSink{user: &user, sendReasonsInEvents: sendReasonsInEvents}
	detail := client.evaluator.Evaluate(*feature, user, eventSink.recordPrerequisiteEvent)
	if detail.Reason.GetKind() == ldreason.EvalReasonError && client.config.LogEvaluationErrors {
		client.config.Loggers.Warnf("flag evaluation for %s failed with error %s, default value was returned",
			key, detail.Reason.GetErrorKind())
	}
	if detail.IsDefaultValue() {
		detail.Value = defaultVal
		detail.VariationIndex = -1
	}
	for _, event := range eventSink.events {
		client.eventProcessor.SendEvent(event)
	}
	return detail, feature, nil
}

func newEvaluationError(jsonValue ldvalue.Value, errorKind ldreason.EvalErrorKind) ldreason.EvaluationDetail {
	return ldreason.EvaluationDetail{
		Value:          jsonValue,
		VariationIndex: -1,
		Reason:         ldreason.NewEvalReasonError(errorKind),
	}
}
