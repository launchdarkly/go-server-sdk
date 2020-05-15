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
	"reflect"
	"strings"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

// Version is the client version.
const Version = "5.0.0"

// LDClient is the LaunchDarkly client. Client instances are thread-safe.
// Applications should instantiate a single instance for the lifetime
// of their application.
type LDClient struct {
	sdkKey                      string
	config                      Config
	eventProcessor              ldevents.EventProcessor
	dataSource                  interfaces.DataSource
	store                       interfaces.DataStore
	evaluator                   ldeval.Evaluator
	dataSourceStatusBroadcaster *internal.DataSourceStatusBroadcaster
	dataSourceStatusProvider    interfaces.DataSourceStatusProvider
	dataStoreStatusBroadcaster  *internal.DataStoreStatusBroadcaster
	dataStoreStatusProvider     interfaces.DataStoreStatusProvider
}

// Implementation of ldeval.PrerequisiteFlagEventRecorder
type clientEvaluatorEventSink struct {
	user         *lduser.User
	events       []ldevents.FeatureRequestEvent
	eventFactory ldevents.EventFactory
}

func (c *clientEvaluatorEventSink) recordPrerequisiteEvent(params ldeval.PrerequisiteFlagEvent) {
	flagProps := ldeval.FlagEventProperties(params.PrerequisiteFlag)
	event := c.eventFactory.NewSuccessfulEvalEvent(flagProps, ldevents.User(*c.user), params.PrerequisiteResult.VariationIndex,
		params.PrerequisiteResult.Value, ldvalue.Null(), params.PrerequisiteResult.Reason, params.TargetFlagKey)
	c.events = append(c.events, event)
}

// Standard event factory when evaluation reasons are not an issue
var defaultEventFactory = ldevents.NewEventFactory(false, nil)

// offlineDataSourceFactory is a stub identical to ldcomponents.ExternalUpdatesOnly(), except that it does not
// log the "daemon mode" message.
type offlineDataSourceFactory struct{}
type offlineDataSource struct{}

func (f offlineDataSourceFactory) CreateDataSource(
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	context.GetLoggers().Info("Started LaunchDarkly client in LDD mode")
	return offlineDataSource{}, nil
}

func (o offlineDataSource) IsInitialized() bool {
	return true
}

func (o offlineDataSource) Close() error {
	return nil
}

func (o offlineDataSource) Start(closeWhenReady chan<- struct{}) {
	close(closeWhenReady)
}

// Initialization errors
var (
	ErrInitializationTimeout = errors.New("timeout encountered waiting for LaunchDarkly client initialization")
	ErrInitializationFailed  = errors.New("LaunchDarkly client initialization failed")
	ErrClientNotInitialized  = errors.New("feature flag evaluation called before LaunchDarkly client initialization completed")
)

// MakeClient creates a new client instance that connects to LaunchDarkly with the default configuration.
//
// The optional duration parameter allows callers to block until the client has connected to LaunchDarkly and is
// properly initialized.
//
// For advanced configuration options, use MakeCustomClient.
func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	return MakeCustomClient(sdkKey, Config{}, waitFor)
}

// MakeCustomClient creates a new client instance that connects to LaunchDarkly with a custom configuration.
//
// The config parameter allows customization of all SDK properties; some of these are represented directly as fields in
// Config, while others are set by builder methods on a more specific configuration object. For instance, to use polling
// mode instead of streaming, configure the polling interval, and use a non-default HTTP timeout for all HTTP requests:
//
//     config := ld.Config{
//         DataSource: ldcomponents.PollingDataSource().PollInterval(45 * time.Minute),
//         Timeout: 4 * time.Second,
//     }
//     client, err := ld.MakeCustomClient(sdkKey, config, 5 * time.Second)
//
// The optional duration parameter allows callers to block until the client has connected to LaunchDarkly and is
// properly initialized.
func MakeCustomClient(sdkKey string, config Config, waitFor time.Duration) (*LDClient, error) {
	closeWhenReady := make(chan struct{})

	config.UserAgent = strings.TrimSpace("GoClient/" + Version + " " + config.UserAgent)

	config.Loggers.Init()
	config.Loggers.Infof("Starting LaunchDarkly client %s", Version)

	eventProcessorFactory := getEventProcessorFactory(config)

	// Do not create a diagnostics manager if diagnostics are disabled, or if we're not using the standard event processor.
	var diagnosticsManager *ldevents.DiagnosticsManager
	if !config.DiagnosticOptOut {
		if reflect.TypeOf(eventProcessorFactory) == reflect.TypeOf(ldcomponents.SendEvents()) {
			diagnosticsManager = createDiagnosticsManager(sdkKey, config, waitFor)
		}
	}

	clientContext := newClientContextImpl(sdkKey, config, config.newHTTPClient, diagnosticsManager)

	client := LDClient{
		sdkKey: sdkKey,
		config: config,
	}

	client.dataStoreStatusBroadcaster = internal.NewDataStoreStatusBroadcaster()
	dataStoreUpdates := internal.NewDataStoreUpdatesImpl(client.dataStoreStatusBroadcaster)
	store, err := getDataStoreFactory(config).CreateDataStore(clientContext, dataStoreUpdates)
	if err != nil {
		return nil, err
	}
	client.store = store

	dataProvider := interfaces.NewDataStoreEvaluatorDataProvider(store, config.Loggers)
	client.evaluator = ldeval.NewEvaluator(dataProvider)
	client.dataStoreStatusProvider = internal.NewDataStoreStatusProviderImpl(store, dataStoreUpdates)

	client.dataSourceStatusBroadcaster = internal.NewDataSourceStatusBroadcaster()
	dataSourceUpdates := internal.NewDataSourceUpdatesImpl(
		store,
		client.dataStoreStatusProvider,
		client.dataSourceStatusBroadcaster,
		config.Loggers,
	)

	client.eventProcessor, err = eventProcessorFactory.CreateEventProcessor(clientContext)
	if err != nil {
		return nil, err
	}

	client.dataSource, err = getDataSourceFactory(config).CreateDataSource(clientContext, dataSourceUpdates)
	if err != nil {
		return nil, err
	}
	client.dataSourceStatusProvider = internal.NewDataSourceStatusProviderImpl(
		client.dataSourceStatusBroadcaster,
		dataSourceUpdates,
	)

	client.dataSource.Start(closeWhenReady)
	if config.Offline {
		config.Loggers.Info("Started LaunchDarkly client in offline mode")
	} else {
		if waitFor > 0 {
			config.Loggers.Infof("Waiting up to %d milliseconds for LaunchDarkly client to start...",
				waitFor/time.Millisecond)
		}
	}
	timeout := time.After(waitFor)
	for {
		select {
		case <-closeWhenReady:
			if !client.dataSource.IsInitialized() {
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

func getDataStoreFactory(config Config) interfaces.DataStoreFactory {
	if config.DataStore == nil {
		return ldcomponents.InMemoryDataStore()
	}
	return config.DataStore
}

func getDataSourceFactory(config Config) interfaces.DataSourceFactory {
	if config.Offline {
		return offlineDataSourceFactory{}
	}
	if config.DataSource == nil {
		return ldcomponents.StreamingDataSource()
	}
	return config.DataSource
}

func getEventProcessorFactory(config Config) interfaces.EventProcessorFactory {
	if config.Offline {
		return ldcomponents.NoEvents()
	}
	if config.Events == nil {
		return ldcomponents.SendEvents()
	}
	return config.Events
}

// Identify reports details about a a user.
func (client *LDClient) Identify(user lduser.User) error {
	if user.GetKey() == "" {
		client.config.Loggers.Warn("Identify called with empty user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	evt := defaultEventFactory.NewIdentifyEvent(ldevents.User(user))
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
	client.eventProcessor.SendEvent(defaultEventFactory.NewCustomEvent(eventName, ldevents.User(user), data, false, 0))
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
	client.eventProcessor.SendEvent(defaultEventFactory.NewCustomEvent(eventName, ldevents.User(user), data, true, metricValue))
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
	return client.dataSource.IsInitialized()
}

// Close shuts down the LaunchDarkly client. After calling this, the LaunchDarkly client
// should no longer be used. The method will block until all pending analytics events (if any)
// been sent.
func (client *LDClient) Close() error {
	client.config.Loggers.Info("Closing LaunchDarkly client")
	_ = client.eventProcessor.Close()
	_ = client.dataSource.Close()
	_ = client.store.Close()
	client.dataSourceStatusBroadcaster.Close()
	client.dataStoreStatusBroadcaster.Close()
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
		if client.store.IsInitialized() {
			client.config.Loggers.Warn("Called AllFlagsState before client initialization; using last known values from data store")
		} else {
			client.config.Loggers.Warn("Called AllFlagsState before client initialization. Data store not available; returning empty state")
			valid = false
		}
	}

	if !valid {
		return FeatureFlagsState{valid: false}
	}

	items, err := client.store.GetAll(interfaces.DataKindFeatures())
	if err != nil {
		client.config.Loggers.Warn("Unable to fetch flags from data store. Returning empty state. Error: " + err.Error())
		return FeatureFlagsState{valid: false}
	}

	state := newFeatureFlagsState()
	clientSideOnly := hasFlagsStateOption(options, ClientSideOnly)
	withReasons := hasFlagsStateOption(options, WithReasons)
	detailsOnlyIfTracked := hasFlagsStateOption(options, DetailsOnlyForTrackedFlags)
	for _, item := range items {
		if item.Item.Item != nil {
			if flag, ok := item.Item.Item.(*ldmodel.FeatureFlag); ok {
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

// GetDataSourceStatusProvider returns an interface for tracking the status of the data source.
//
// The data source is the mechanism that the SDK uses to get feature flag configurations, such as a
// streaming connection (the default) or poll requests. The DataSourceStatusProvider has methods
// for checking whether the data source is (as far as the SDK knows) currently operational and tracking
// changes in this status.
func (client *LDClient) GetDataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return client.dataSourceStatusProvider
}

// GetDataStoreStatusProvider returns an interface for tracking the status of a persistent data store.
//
// The DataStoreStatusProvider has methods for checking whether the data store is (as far as the SDK
// SDK knows) currently operational, tracking changes in this status, and getting cache statistics. These
// are only relevant for a persistent data store; if you are using an in-memory data store, then this
// method will always report that the store is operational.
func (client *LDClient) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return client.dataStoreStatusProvider
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
	eventFactory := ldevents.NewEventFactory(sendReasonsInEvents, nil)
	result, flag, err := client.evaluateInternal(key, user, defaultVal, eventFactory)
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
		evt = eventFactory.NewUnknownFlagEvent(key, ldevents.User(user), defaultVal, result.Reason) //nolint
	} else {
		flagProps := ldeval.FlagEventProperties(*flag)
		evt = eventFactory.NewSuccessfulEvalEvent(flagProps, ldevents.User(user), result.VariationIndex, result.Value, defaultVal,
			result.Reason, "")
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
	eventFactory ldevents.EventFactory,
) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
	if user.GetKey() == "" {
		client.config.Loggers.Warnf("User.Key is blank when evaluating flag: %s. Flag evaluation will proceed, but the user will not be stored in LaunchDarkly.", key)
	}

	var feature *ldmodel.FeatureFlag
	var storeErr error
	var ok bool

	evalErrorResult := func(errKind ldreason.EvalErrorKind, flag *ldmodel.FeatureFlag, err error) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
		detail := newEvaluationError(defaultVal, errKind)
		if client.config.LogEvaluationErrors {
			client.config.Loggers.Warn(err)
		}
		return detail, flag, err
	}

	if !client.Initialized() {
		if client.store.IsInitialized() {
			client.config.Loggers.Warn("Feature flag evaluation called before LaunchDarkly client initialization completed; using last known values from data store")
		} else {
			return evalErrorResult(ldreason.EvalErrorClientNotReady, nil, ErrClientNotInitialized)
		}
	}

	itemDesc, storeErr := client.store.Get(interfaces.DataKindFeatures(), key)

	if storeErr != nil {
		client.config.Loggers.Errorf("Encountered error fetching feature from store: %+v", storeErr)
		detail := newEvaluationError(defaultVal, ldreason.EvalErrorException)
		return detail, nil, storeErr
	}

	if itemDesc.Item != nil {
		feature, ok = itemDesc.Item.(*ldmodel.FeatureFlag)
		if !ok {
			return evalErrorResult(ldreason.EvalErrorException, nil,
				fmt.Errorf("unexpected data type (%T) found in store for feature key: %s. Returning default value", itemDesc.Item, key))
		}
	} else {
		return evalErrorResult(ldreason.EvalErrorFlagNotFound, nil,
			fmt.Errorf("unknown feature key: %s. Verify that this feature key exists. Returning default value", key))
	}

	eventSink := clientEvaluatorEventSink{user: &user, eventFactory: eventFactory}
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
