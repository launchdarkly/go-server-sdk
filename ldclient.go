package ldclient

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	ldeval "gopkg.in/launchdarkly/go-server-sdk-evaluation.v1"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces/flagstate"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datakinds"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datasource"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/datastore"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents/ldstoreimpl"
)

// Version is the SDK version.
const Version = internal.SDKVersion

// LDClient is the LaunchDarkly client.
//
// This object evaluates feature flags, generates analytics events, and communicates with
// LaunchDarkly services. Applications should instantiate a single instance for the lifetime
// of their application and share it wherever feature flags need to be evaluated; all LDClient
// methods are safe to be called concurrently from multiple goroutines.
//
// Some advanced client features are grouped together in API facades that are accessed through
// an LDClient method, such as GetDataSourceStatusProvider().
//
// When an application is shutting down or no longer needs to use the LDClient instance, it
// should call Close() to ensure that all of its connections and goroutines are shut down and
// that any pending analytics events have been delivered.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go
type LDClient struct {
	sdkKey                      string
	loggers                     ldlog.Loggers
	eventProcessor              ldevents.EventProcessor
	dataSource                  interfaces.DataSource
	store                       interfaces.DataStore
	evaluator                   ldeval.Evaluator
	dataSourceStatusBroadcaster *internal.DataSourceStatusBroadcaster
	dataSourceStatusProvider    interfaces.DataSourceStatusProvider
	dataStoreStatusBroadcaster  *internal.DataStoreStatusBroadcaster
	dataStoreStatusProvider     interfaces.DataStoreStatusProvider
	flagChangeEventBroadcaster  *internal.FlagChangeEventBroadcaster
	flagTracker                 interfaces.FlagTracker
	eventsDefault               eventsScope
	eventsWithReasons           eventsScope
	withEventsDisabled          interfaces.LDClientInterface
	logEvaluationErrors         bool
	offline                     bool
}

// Initialization errors
var (
	// MakeClient and MakeCustomClient will return this error if the SDK was not able to establish a
	// LaunchDarkly connection within the specified time interval. In this case, the LDClient will still
	// continue trying to connect in the background.
	ErrInitializationTimeout = errors.New("timeout encountered waiting for LaunchDarkly client initialization")

	// MakeClient and MakeCustomClient will return this error if the SDK detected an error that makes it
	// impossible for a LaunchDarkly connection to succeed. Currently, the only such condition is if the
	// SDK key is invalid, since an invalid SDK key will never become valid.
	ErrInitializationFailed = errors.New("LaunchDarkly client initialization failed")

	// This error is returned by the Variation/VariationDetail methods if feature flags are not available
	// because the client has not successfully initialized. In this case, the result value will be whatever
	// default value was specified by the application.
	ErrClientNotInitialized = errors.New("feature flag evaluation called before LaunchDarkly client initialization completed") //nolint:lll
)

// MakeClient creates a new client instance that connects to LaunchDarkly with the default configuration.
//
// For advanced configuration options, use MakeCustomClient. Calling MakeClient is exactly equivalent to
// calling MakeCustomClient with the config parameter set to an empty value, ld.Config{}.
//
// Unless it is configured to be offline with Config.Offline or ldcomponents.ExternalUpdatesOnly(), the client
// will begin attempting to connect to LaunchDarkly as soon as you call this constructor. The constructor will
// return when it successfully connects, or when the timeout set by the waitFor parameter expires, whichever
// comes first.
//
// If the connection succeeded, the first return value is the client instance, and the error value is nil.
//
// If the timeout elapsed without a successful connection, it still returns a client instance-- in an
// uninitialized state, where feature flags will return default values-- and the error value is
// ErrInitializationTimeout. In this case, it will still continue trying to connect in the background.
//
// If there was an unrecoverable error such that it cannot succeed by retrying-- for instance, the SDK key is
// invalid-- it will return a client instance in an uninitialized state, and the error value is
// ErrInitializationFailed.
//
// If you set waitFor to zero, the function will return immediately after creating the client instance, and
// do any further initialization in the background.
//
// The only time it returns nil instead of a client instance is if the client cannot be created at all due to
// an invalid configuration. This is rare, but could happen if for instance you specified a custom TLS
// certificate file that did not contain a valid certificate.
//
// For more about the difference between an initialized and uninitialized client, and other ways to monitor
// the client's status, see LDClient.Initialized() and LDClient.GetDataSourceStatusProvider().
func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	// COVERAGE: this constructor cannot be called in unit tests because it uses the default base
	// URI and will attempt to make a live connection to LaunchDarkly.
	return MakeCustomClient(sdkKey, Config{}, waitFor)
}

// MakeCustomClient creates a new client instance that connects to LaunchDarkly with a custom configuration.
//
// The config parameter allows customization of all SDK properties; some of these are represented directly as
// fields in Config, while others are set by builder methods on a more specific configuration object. See
// Config for details.
//
// Unless it is configured to be offline with Config.Offline or ldcomponents.ExternalUpdatesOnly(), the client
// will begin attempting to connect to LaunchDarkly as soon as you call this constructor. The constructor will
// return when it successfully connects, or when the timeout set by the waitFor parameter expires, whichever
// comes first.
//
// If the connection succeeded, the first return value is the client instance, and the error value is nil.
//
// If the timeout elapsed without a successful connection, it still returns a client instance-- in an
// uninitialized state, where feature flags will return default values-- and the error value is
// ErrInitializationTimeout. In this case, it will still continue trying to connect in the background.
//
// If there was an unrecoverable error such that it cannot succeed by retrying-- for instance, the SDK key is
// invalid-- it will return a client instance in an uninitialized state, and the error value is
// ErrInitializationFailed.
//
// If you set waitFor to zero, the function will return immediately after creating the client instance, and
// do any further initialization in the background.
//
// The only time it returns nil instead of a client instance is if the client cannot be created at all due to
// an invalid configuration. This is rare, but could happen if for instance you specified a custom TLS
// certificate file that did not contain a valid certificate.
//
// For more about the difference between an initialized and uninitialized client, and other ways to monitor
// the client's status, see LDClient.Initialized() and LDClient.GetDataSourceStatusProvider().
func MakeCustomClient(sdkKey string, config Config, waitFor time.Duration) (*LDClient, error) {
	closeWhenReady := make(chan struct{})

	eventProcessorFactory := getEventProcessorFactory(config)

	// Do not create a diagnostics manager if diagnostics are disabled, or if we're not using the standard event processor.
	var diagnosticsManager *ldevents.DiagnosticsManager
	if !config.DiagnosticOptOut {
		if reflect.TypeOf(eventProcessorFactory) == reflect.TypeOf(ldcomponents.SendEvents()) {
			diagnosticsManager = createDiagnosticsManager(sdkKey, config, waitFor)
		}
	}

	clientContext, err := newClientContextFromConfig(sdkKey, config, diagnosticsManager)
	if err != nil {
		return nil, err
	}

	loggers := clientContext.GetLogging().GetLoggers()
	loggers.Infof("Starting LaunchDarkly client %s", Version)

	client := &LDClient{
		sdkKey:              sdkKey,
		loggers:             loggers,
		logEvaluationErrors: clientContext.GetLogging().IsLogEvaluationErrors(),
		offline:             config.Offline,
	}

	client.dataStoreStatusBroadcaster = internal.NewDataStoreStatusBroadcaster()
	dataStoreUpdates := datastore.NewDataStoreUpdatesImpl(client.dataStoreStatusBroadcaster)
	store, err := getDataStoreFactory(config).CreateDataStore(clientContext, dataStoreUpdates)
	if err != nil {
		return nil, err
	}
	client.store = store

	dataProvider := ldstoreimpl.NewDataStoreEvaluatorDataProvider(store, loggers)
	client.evaluator = ldeval.NewEvaluator(dataProvider)
	client.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(store, dataStoreUpdates)

	client.dataSourceStatusBroadcaster = internal.NewDataSourceStatusBroadcaster()
	client.flagChangeEventBroadcaster = internal.NewFlagChangeEventBroadcaster()
	dataSourceUpdates := datasource.NewDataSourceUpdatesImpl(
		store,
		client.dataStoreStatusProvider,
		client.dataSourceStatusBroadcaster,
		client.flagChangeEventBroadcaster,
		clientContext.GetLogging().GetLogDataSourceOutageAsErrorAfter(),
		loggers,
	)

	client.eventProcessor, err = eventProcessorFactory.CreateEventProcessor(clientContext)
	if err != nil {
		return nil, err
	}
	if isNullEventProcessorFactory(eventProcessorFactory) {
		client.eventsDefault = newDisabledEventsScope()
		client.eventsWithReasons = newDisabledEventsScope()
	} else {
		client.eventsDefault = newEventsScope(client, false)
		client.eventsWithReasons = newEventsScope(client, true)
	}
	// Pre-create the WithEventsDisabled object so that if an application ends up calling WithEventsDisabled
	// frequently, it won't be causing an allocation each time.
	client.withEventsDisabled = newClientEventsDisabledDecorator(client)

	dataSource, err := createDataSource(config, clientContext, dataSourceUpdates)
	client.dataSource = dataSource
	if err != nil {
		return nil, err
	}
	client.dataSourceStatusProvider = datasource.NewDataSourceStatusProviderImpl(
		client.dataSourceStatusBroadcaster,
		dataSourceUpdates,
	)

	client.flagTracker = internal.NewFlagTrackerImpl(
		client.flagChangeEventBroadcaster,
		func(flagKey string, user lduser.User, defaultValue ldvalue.Value) ldvalue.Value {
			value, _ := client.JSONVariation(flagKey, user, defaultValue)
			return value
		},
	)

	client.dataSource.Start(closeWhenReady)
	if waitFor > 0 && client.dataSource != datasource.NewNullDataSource() {
		loggers.Infof("Waiting up to %d milliseconds for LaunchDarkly client to start...",
			waitFor/time.Millisecond)
		timeout := time.After(waitFor)
		for {
			select {
			case <-closeWhenReady:
				if !client.dataSource.IsInitialized() {
					loggers.Warn("LaunchDarkly client initialization failed")
					return client, ErrInitializationFailed
				}

				loggers.Info("Initialized LaunchDarkly client")
				return client, nil
			case <-timeout:
				loggers.Warn("Timeout encountered waiting for LaunchDarkly client initialization")
				go func() { <-closeWhenReady }() // Don't block the DataSource when not waiting
				return client, ErrInitializationTimeout
			}
		}
	}
	go func() { <-closeWhenReady }() // Don't block the DataSource when not waiting
	return client, nil
}

func getDataStoreFactory(config Config) interfaces.DataStoreFactory {
	if config.DataStore == nil {
		return ldcomponents.InMemoryDataStore()
	}
	return config.DataStore
}

func createDataSource(
	config Config,
	context interfaces.ClientContext,
	dataSourceUpdates interfaces.DataSourceUpdates,
) (interfaces.DataSource, error) {
	if config.Offline {
		context.GetLogging().GetLoggers().Info("Starting LaunchDarkly client in offline mode")
		dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
		return datasource.NewNullDataSource(), nil
	}
	factory := config.DataSource
	if factory == nil {
		// COVERAGE: can't cause this condition in unit tests because it would try to connect to production LD
		factory = ldcomponents.StreamingDataSource()
	}
	return factory.CreateDataSource(context, dataSourceUpdates)
}

// Identify reports details about a user.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#identify
func (client *LDClient) Identify(user lduser.User) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if user.GetKey() == "" {
		client.loggers.Warn("Identify called with empty user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	evt := client.eventsDefault.factory.NewIdentifyEvent(ldevents.User(user))
	client.eventProcessor.RecordIdentifyEvent(evt)
	return nil
}

// TrackEvent reports that a user has performed an event.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard. If you want to associate additional data with this event, use TrackData
// or TrackMetric.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#track
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
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#track
func (client *LDClient) TrackData(eventName string, user lduser.User, data ldvalue.Value) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if user.GetKey() == "" {
		client.loggers.Warn("Track called with empty user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	client.eventProcessor.RecordCustomEvent(
		client.eventsDefault.factory.NewCustomEvent(
			eventName,
			ldevents.User(user),
			data,
			false,
			0,
		))
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
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#track
func (client *LDClient) TrackMetric(eventName string, user lduser.User, metricValue float64, data ldvalue.Value) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if user.GetKey() == "" {
		client.loggers.Warn("Track called with empty/nil user key!")
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	client.eventProcessor.RecordCustomEvent(
		client.eventsDefault.factory.NewCustomEvent(
			eventName,
			ldevents.User(user),
			data,
			true,
			metricValue,
		))
	return nil
}

// IsOffline returns whether the LaunchDarkly client is in offline mode.
//
// This is only true if you explicitly set the Config.Offline property to true, to force the client to
// be offline. It does not mean that the client is having a problem connecting to LaunchDarkly. To detect
// the status of a client that is configured to be online, use Initialized() or
// GetDataSourceStatusProvider().
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#offline-mode
func (client *LDClient) IsOffline() bool {
	return client.offline
}

// SecureModeHash generates the secure mode hash value for a user.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#secure-mode-hash
func (client *LDClient) SecureModeHash(user lduser.User) string {
	key := []byte(client.sdkKey)
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(user.GetKey()))
	return hex.EncodeToString(h.Sum(nil))
}

// Initialized returns whether the LaunchDarkly client is initialized.
//
// If this value is true, it means the client has succeeded at some point in connecting to LaunchDarkly and
// has received feature flag data. It could still have encountered a connection problem after that point, so
// this does not guarantee that the flags are up to date; if you need to know its status in more detail, use
// GetDataSourceStatusProvider.
//
// If this value is false, it means the client has not yet connected to LaunchDarkly, or has permanently
// failed. See MakeClient for the reasons that this could happen. In this state, feature flag evaluations
// will always return default values-- unless you are using a database integration and feature flags had
// already been stored in the database by a successfully connected SDK in the past. You can use
// GetDataSourceStatusProvider to get information on errors, or to wait for a successful retry.
func (client *LDClient) Initialized() bool {
	return client.dataSource.IsInitialized()
}

// Close shuts down the LaunchDarkly client. After calling this, the LaunchDarkly client
// should no longer be used. The method will block until all pending analytics events (if any)
// been sent.
func (client *LDClient) Close() error {
	client.loggers.Info("Closing LaunchDarkly client")
	_ = client.eventProcessor.Close()
	_ = client.dataSource.Close()
	_ = client.store.Close()
	client.dataSourceStatusBroadcaster.Close()
	client.dataStoreStatusBroadcaster.Close()
	client.flagChangeEventBroadcaster.Close()
	return nil
}

// Flush tells the client that all pending analytics events (if any) should be delivered as soon
// as possible. Flushing is asynchronous, so this method will return before it is complete.
// However, if you call Close(), events are guaranteed to be sent before that method returns.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#flush
func (client *LDClient) Flush() {
	client.eventProcessor.Flush()
}

// AllFlagsState returns an object that encapsulates the state of all feature flags for a given user.
// This includes the flag values, and also metadata that can be used on the front end.
//
// The most common use case for this method is to bootstrap a set of client-side feature flags from a
// back-end service.
//
// You may pass any combination of flagstate.ClientSideOnly, flagstate.WithReasons, and
// flagstate.DetailsOnlyForTrackedFlags as optional parameters to control what data is included.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#all-flags
func (client *LDClient) AllFlagsState(user lduser.User, options ...flagstate.Option) flagstate.AllFlags {
	valid := true
	if client.IsOffline() {
		client.loggers.Warn("Called AllFlagsState in offline mode. Returning empty state")
		valid = false
	} else if !client.Initialized() {
		if client.store.IsInitialized() {
			client.loggers.Warn("Called AllFlagsState before client initialization; using last known values from data store")
		} else {
			client.loggers.Warn("Called AllFlagsState before client initialization. Data store not available; returning empty state") //nolint:lll
			valid = false
		}
	}

	if !valid {
		return flagstate.AllFlags{}
	}

	items, err := client.store.GetAll(datakinds.Features)
	if err != nil {
		client.loggers.Warn("Unable to fetch flags from data store. Returning empty state. Error: " + err.Error())
		return flagstate.AllFlags{}
	}

	clientSideOnly := false
	for _, o := range options {
		if o == flagstate.OptionClientSideOnly() {
			clientSideOnly = true
			break
		}
	}

	state := flagstate.NewAllFlagsBuilder(options...)
	for _, item := range items {
		if item.Item.Item != nil {
			if flag, ok := item.Item.Item.(*ldmodel.FeatureFlag); ok {
				if clientSideOnly && !flag.ClientSideAvailability.UsingEnvironmentID {
					continue
				}
				result := client.evaluator.Evaluate(flag, user, nil)
				state.AddFlag(
					item.Key,
					flagstate.FlagState{
						Value:                result.Value,
						Variation:            result.VariationIndex,
						Reason:               result.Reason,
						Version:              flag.Version,
						TrackEvents:          flag.TrackEvents,
						DebugEventsUntilDate: flag.DebugEventsUntilDate,
					},
				)
			}
		}
	}

	return state.Build()
}

// BoolVariation returns the value of a boolean feature flag for a given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variation
func (client *LDClient) BoolVariation(key string, user lduser.User, defaultVal bool) (bool, error) {
	detail, err := client.variation(key, user, ldvalue.Bool(defaultVal), true, client.eventsDefault)
	return detail.Value.BoolValue(), err
}

// BoolVariationDetail is the same as BoolVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variationdetail
func (client *LDClient) BoolVariationDetail(
	key string,
	user lduser.User,
	defaultVal bool,
) (bool, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Bool(defaultVal), true, client.eventsWithReasons)
	return detail.Value.BoolValue(), detail, err
}

// IntVariation returns the value of a feature flag (whose variations are integers) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// If the flag variation has a numeric value that is not an integer, it is rounded toward zero (truncated).
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variation
func (client *LDClient) IntVariation(key string, user lduser.User, defaultVal int) (int, error) {
	detail, err := client.variation(key, user, ldvalue.Int(defaultVal), true, client.eventsDefault)
	return detail.Value.IntValue(), err
}

// IntVariationDetail is the same as IntVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variationdetail
func (client *LDClient) IntVariationDetail(
	key string,
	user lduser.User,
	defaultVal int,
) (int, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Int(defaultVal), true, client.eventsWithReasons)
	return detail.Value.IntValue(), detail, err
}

// Float64Variation returns the value of a feature flag (whose variations are floats) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variation
func (client *LDClient) Float64Variation(key string, user lduser.User, defaultVal float64) (float64, error) {
	detail, err := client.variation(key, user, ldvalue.Float64(defaultVal), true, client.eventsDefault)
	return detail.Value.Float64Value(), err
}

// Float64VariationDetail is the same as Float64Variation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variationdetail
func (client *LDClient) Float64VariationDetail(
	key string,
	user lduser.User,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.Float64(defaultVal), true, client.eventsWithReasons)
	return detail.Value.Float64Value(), detail, err
}

// StringVariation returns the value of a feature flag (whose variations are strings) for the given user.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and has
// no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variation
func (client *LDClient) StringVariation(key string, user lduser.User, defaultVal string) (string, error) {
	detail, err := client.variation(key, user, ldvalue.String(defaultVal), true, client.eventsDefault)
	return detail.Value.StringValue(), err
}

// StringVariationDetail is the same as StringVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variationdetail
func (client *LDClient) StringVariationDetail(
	key string,
	user lduser.User,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, ldvalue.String(defaultVal), true, client.eventsWithReasons)
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
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variation
func (client *LDClient) JSONVariation(key string, user lduser.User, defaultVal ldvalue.Value) (ldvalue.Value, error) {
	detail, err := client.variation(key, user, defaultVal, false, client.eventsDefault)
	return detail.Value, err
}

// JSONVariationDetail is the same as JSONVariation, but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go#variationdetail
func (client *LDClient) JSONVariationDetail(
	key string,
	user lduser.User,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, err := client.variation(key, user, defaultVal, false, client.eventsWithReasons)
	return detail.Value, detail, err
}

// GetDataSourceStatusProvider returns an interface for tracking the status of the data source.
//
// The data source is the mechanism that the SDK uses to get feature flag configurations, such as a
// streaming connection (the default) or poll requests. The DataSourceStatusProvider has methods
// for checking whether the data source is (as far as the SDK knows) currently operational and tracking
// changes in this status.
//
// See the DataSourceStatusProvider interface for more about this functionality.
func (client *LDClient) GetDataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return client.dataSourceStatusProvider
}

// GetDataStoreStatusProvider returns an interface for tracking the status of a persistent data store.
//
// The DataStoreStatusProvider has methods for checking whether the data store is (as far as the SDK
// SDK knows) currently operational, tracking changes in this status, and getting cache statistics. These
// are only relevant for a persistent data store; if you are using an in-memory data store, then this
// method will always report that the store is operational.
//
// See the DataStoreStatusProvider interface for more about this functionality.
func (client *LDClient) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return client.dataStoreStatusProvider
}

// GetFlagTracker returns an interface for tracking changes in feature flag configurations.
//
// See the FlagTracker interface for more about this functionality.
func (client *LDClient) GetFlagTracker() interfaces.FlagTracker {
	return client.flagTracker
}

// WithEventsDisabled returns a decorator for the LDClient that implements the same basic operations
// but will not generate any analytics events.
//
// If events were already disabled, this is just the same LDClient. Otherwise, it is an object whose
// Variation methods use the same LDClient to evaluate feature flags, but without generating any
// events, and whose Identify/Track/Custom methods do nothing. Neither evaluation counts nor user
// properties will be sent to LaunchDarkly for any operations done with this object.
//
// You can use this to suppress events within some particular area of your code where you do not want
// evaluations to affect your dashboard statistics, or do not want to incur the overhead of processing
// the events.
//
// Note that if the original client configuration already had events disabled
// (config.Events = ldcomponents.NoEvents()), you cannot re-enable them with this method. It is only
// useful for temporarily disabling events on a client that had them enabled.
func (client *LDClient) WithEventsDisabled(disabled bool) interfaces.LDClientInterface {
	if !disabled || client.eventsDefault.disabled {
		return client
	}
	return client.withEventsDisabled
}

// Generic method for evaluating a feature flag for a given user.
func (client *LDClient) variation(
	key string,
	user lduser.User,
	defaultVal ldvalue.Value,
	checkType bool,
	eventsScope eventsScope,
) (ldreason.EvaluationDetail, error) {
	if client.IsOffline() {
		return newEvaluationError(defaultVal, ldreason.EvalErrorClientNotReady), nil
	}
	result, flag, err := client.evaluateInternal(key, user, defaultVal, eventsScope)
	if err != nil {
		result.Value = defaultVal
		result.VariationIndex = ldvalue.OptionalInt{}
	} else if checkType && defaultVal.Type() != ldvalue.NullType && result.Value.Type() != defaultVal.Type() {
		result = newEvaluationError(defaultVal, ldreason.EvalErrorWrongType)
	}

	if !eventsScope.disabled {
		var evt ldevents.FeatureRequestEvent
		if flag == nil {
			evt = eventsScope.factory.NewUnknownFlagEvent(key, ldevents.User(user), defaultVal, result.Reason)
		} else {
			evt = eventsScope.factory.NewEvalEvent(
				flag,
				ldevents.User(user),
				result,
				defaultVal,
				"",
			)
		}
		client.eventProcessor.RecordFeatureRequestEvent(evt)
	}

	return result, err
}

// Performs all the steps of evaluation except for sending the feature request event (the main one;
// events for prerequisites will be sent).
func (client *LDClient) evaluateInternal(
	key string,
	user lduser.User,
	defaultVal ldvalue.Value,
	eventsScope eventsScope,
) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
	// THIS IS A HIGH-TRAFFIC CODE PATH so performance tuning is important. Please see CONTRIBUTING.md for guidelines
	// to keep in mind during any changes to the evaluation logic.

	var feature *ldmodel.FeatureFlag
	var storeErr error
	var ok bool

	evalErrorResult := func(
		errKind ldreason.EvalErrorKind,
		flag *ldmodel.FeatureFlag,
		err error,
	) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
		detail := newEvaluationError(defaultVal, errKind)
		if client.logEvaluationErrors {
			client.loggers.Warn(err)
		}
		return detail, flag, err
	}

	if !client.Initialized() {
		if client.store.IsInitialized() {
			client.loggers.Warn("Feature flag evaluation called before LaunchDarkly client initialization completed; using last known values from data store") //nolint:lll
		} else {
			return evalErrorResult(ldreason.EvalErrorClientNotReady, nil, ErrClientNotInitialized)
		}
	}

	itemDesc, storeErr := client.store.Get(datakinds.Features, key)

	if storeErr != nil {
		client.loggers.Errorf("Encountered error fetching feature from store: %+v", storeErr)
		detail := newEvaluationError(defaultVal, ldreason.EvalErrorException)
		return detail, nil, storeErr
	}

	if itemDesc.Item != nil {
		feature, ok = itemDesc.Item.(*ldmodel.FeatureFlag)
		if !ok {
			return evalErrorResult(ldreason.EvalErrorException, nil,
				fmt.Errorf(
					"unexpected data type (%T) found in store for feature key: %s. Returning default value",
					itemDesc.Item,
					key,
				))
		}
	} else {
		return evalErrorResult(ldreason.EvalErrorFlagNotFound, nil,
			fmt.Errorf("unknown feature key: %s. Verify that this feature key exists. Returning default value", key))
	}

	detail := client.evaluator.Evaluate(feature, user, eventsScope.prerequisiteEventRecorder)
	if detail.Reason.GetKind() == ldreason.EvalReasonError && client.logEvaluationErrors {
		client.loggers.Warnf("Flag evaluation for %s failed with error %s, default value was returned",
			key, detail.Reason.GetErrorKind())
	}
	if detail.IsDefaultValue() {
		detail.Value = defaultVal
	}
	return detail, feature, nil
}

func newEvaluationError(jsonValue ldvalue.Value, errorKind ldreason.EvalErrorKind) ldreason.EvaluationDetail {
	return ldreason.EvaluationDetail{
		Value:  jsonValue,
		Reason: ldreason.NewEvalReasonError(errorKind),
	}
}
