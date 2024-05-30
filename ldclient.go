package ldclient

import (
	gocontext "context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldmigration"
	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldeval "github.com/launchdarkly/go-server-sdk-evaluation/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	ldevents "github.com/launchdarkly/go-server-sdk/ldevents/v4"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/bigsegments"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/internal/hooks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
)

// Version is the SDK version.
const Version = internal.SDKVersion

// highWaitForSeconds is the initialization wait time threshold that we will log a warning message
const highWaitForSeconds = 60

const (
	boolVarFuncName   = "LDClient.BoolVariation"
	intVarFuncName    = "LDClient.IntVariation"
	floatVarFuncName  = "LDClient.Float64Variation"
	stringVarFuncName = "LDClient.StringVariation"
	jsonVarFuncName   = "LDClient.JSONVariation"

	boolVarExFuncName   = "LDClient.BoolVariationCtx"
	intVarExFuncName    = "LDClient.IntVariationCtx"
	floatVarExFuncName  = "LDClient.Float64VariationCtx"
	stringVarExFuncName = "LDClient.StringVariationCtx"
	jsonVarExFuncName   = "LDClient.JSONVariationCtx"

	boolVarDetailFuncName   = "LDClient.BoolVariationDetail"
	intVarDetailFuncName    = "LDClient.IntVariationDetail"
	floatVarDetailFuncName  = "LDClient.Float64VariationDetail"
	stringVarDetailFuncName = "LDClient.StringVariationDetail"
	jsonVarDetailFuncName   = "LDClient.JSONVariationDetail"

	boolVarDetailExFuncName   = "LDClient.BoolVariationDetailCtx"
	intVarDetailExFuncName    = "LDClient.IntVariationDetailCtx"
	floatVarDetailExFuncName  = "LDClient.Float64VariationDetailCtx"
	stringVarDetailExFuncName = "LDClient.StringVariationDetailCtx"
	jsonVarDetailExFuncName   = "LDClient.JSONVariationDetailCtx"

	migrationVarFuncName   = "LDClient.MigrationVariation"
	migrationVarExFuncName = "LDClient.MigrationVariationCtx"
)

// LDClient is the LaunchDarkly client.
//
// This object evaluates feature flags, generates analytics events, and communicates with
// LaunchDarkly services. Applications should instantiate a single instance for the lifetime
// of their application and share it wherever feature flags need to be evaluated; all LDClient
// methods are safe to be called concurrently from multiple goroutines.
//
// Some advanced client features are grouped together in API facades that are accessed through
// an LDClient method, such as [LDClient.GetDataSourceStatusProvider].
//
// When an application is shutting down or no longer needs to use the LDClient instance, it
// should call [LDClient.Close] to ensure that all of its connections and goroutines are shut down and
// that any pending analytics events have been delivered.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/server-side/go
type LDClient struct {
	sdkKey                           string
	loggers                          ldlog.Loggers
	eventProcessor                   ldevents.EventProcessor
	dataSource                       subsystems.DataSource
	store                            subsystems.DataStore
	evaluator                        ldeval.Evaluator
	dataSourceStatusBroadcaster      *internal.Broadcaster[interfaces.DataSourceStatus]
	dataSourceStatusProvider         interfaces.DataSourceStatusProvider
	dataStoreStatusBroadcaster       *internal.Broadcaster[interfaces.DataStoreStatus]
	dataStoreStatusProvider          interfaces.DataStoreStatusProvider
	flagChangeEventBroadcaster       *internal.Broadcaster[interfaces.FlagChangeEvent]
	flagTracker                      interfaces.FlagTracker
	bigSegmentStoreStatusBroadcaster *internal.Broadcaster[interfaces.BigSegmentStoreStatus]
	bigSegmentStoreStatusProvider    interfaces.BigSegmentStoreStatusProvider
	bigSegmentStoreWrapper           *ldstoreimpl.BigSegmentStoreWrapper
	eventsDefault                    eventsScope
	eventsWithReasons                eventsScope
	withEventsDisabled               interfaces.LDClientInterface
	logEvaluationErrors              bool
	offline                          bool
	hookRunner                       *hooks.Runner
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
// For advanced configuration options, use [MakeCustomClient]. Calling MakeClient is exactly equivalent to
// calling MakeCustomClient with the config parameter set to an empty value, ld.Config{}.
//
// The client will begin attempting to connect to LaunchDarkly as soon as you call this constructor. The
// constructor will return when it successfully connects, or when the timeout set by the waitFor parameter
// expires, whichever comes first.
//
// If the connection succeeded, the first return value is the client instance, and the error value is nil.
//
// If the timeout elapsed without a successful connection, it still returns a client instance-- in an
// uninitialized state, where feature flags will return default values-- and the error value is
// [ErrInitializationTimeout]. In this case, it will still continue trying to connect in the background.
//
// If there was an unrecoverable error such that it cannot succeed by retrying-- for instance, the SDK key is
// invalid-- it will return a client instance in an uninitialized state, and the error value is
// [ErrInitializationFailed].
//
// If you set waitFor to zero, the function will return immediately after creating the client instance, and
// do any further initialization in the background.
//
// The only time it returns nil instead of a client instance is if the client cannot be created at all due to
// an invalid configuration. This is rare, but could happen if for instance you specified a custom TLS
// certificate file that did not contain a valid certificate.
//
// For more about the difference between an initialized and uninitialized client, and other ways to monitor
// the client's status, see [LDClient.Initialized] and [LDClient.GetDataSourceStatusProvider].
func MakeClient(sdkKey string, waitFor time.Duration) (*LDClient, error) {
	// COVERAGE: this constructor cannot be called in unit tests because it uses the default base
	// URI and will attempt to make a live connection to LaunchDarkly.
	return MakeCustomClient(sdkKey, Config{}, waitFor)
}

// MakeCustomClient creates a new client instance that connects to LaunchDarkly with a custom configuration.
//
// The config parameter allows customization of all SDK properties; some of these are represented directly as
// fields in Config, while others are set by builder methods on a more specific configuration object. See
// [Config] for details.
//
// Unless it is configured to be offline with Config.Offline or [ldcomponents.ExternalUpdatesOnly], the client
// will begin attempting to connect to LaunchDarkly as soon as you call this constructor. The constructor will
// return when it successfully connects, or when the timeout set by the waitFor parameter expires, whichever
// comes first.
//
// If the connection succeeded, the first return value is the client instance, and the error value is nil.
//
// If the timeout elapsed without a successful connection, it still returns a client instance-- in an
// uninitialized state, where feature flags will return default values-- and the error value is
// [ErrInitializationTimeout]. In this case, it will still continue trying to connect in the background.
//
// If there was an unrecoverable error such that it cannot succeed by retrying-- for instance, the SDK key is
// invalid-- it will return a client instance in an uninitialized state, and the error value is
// [ErrInitializationFailed].
//
// If you set waitFor to zero, the function will return immediately after creating the client instance, and
// do any further initialization in the background.
//
// The only time it returns nil instead of a client instance is if the client cannot be created at all due to
// an invalid configuration. This is rare, but could happen if for instance you specified a custom TLS
// certificate file that did not contain a valid certificate.
//
// For more about the difference between an initialized and uninitialized client, and other ways to monitor
// the client's status, see [LDClient.Initialized] and [LDClient.GetDataSourceStatusProvider].
func MakeCustomClient(sdkKey string, config Config, waitFor time.Duration) (*LDClient, error) {
	// Ensure that any intermediate components we create will be disposed of if we return an error
	client := &LDClient{sdkKey: sdkKey}
	clientValid := false
	defer func() {
		if !clientValid {
			_ = client.Close()
		}
	}()

	closeWhenReady := make(chan struct{})

	eventProcessorFactory := getEventProcessorFactory(config)

	clientContext, err := newClientContextFromConfig(sdkKey, config)
	if err != nil {
		return nil, err
	}

	// Do not create a diagnostics manager if diagnostics are disabled, or if we're not using the standard event processor.
	if !config.DiagnosticOptOut {
		if reflect.TypeOf(eventProcessorFactory) == reflect.TypeOf(ldcomponents.SendEvents()) {
			clientContext.DiagnosticsManager = createDiagnosticsManager(clientContext, sdkKey, config, waitFor)
		}
	}

	loggers := clientContext.GetLogging().Loggers
	loggers.Infof("Starting LaunchDarkly client %s", Version)

	client.loggers = loggers
	client.logEvaluationErrors = clientContext.GetLogging().LogEvaluationErrors

	client.offline = config.Offline

	client.dataStoreStatusBroadcaster = internal.NewBroadcaster[interfaces.DataStoreStatus]()
	dataStoreUpdateSink := datastore.NewDataStoreUpdateSinkImpl(client.dataStoreStatusBroadcaster)
	storeFactory := config.DataStore
	if storeFactory == nil {
		storeFactory = ldcomponents.InMemoryDataStore()
	}
	clientContextWithDataStoreUpdateSink := clientContext
	clientContextWithDataStoreUpdateSink.DataStoreUpdateSink = dataStoreUpdateSink
	store, err := storeFactory.Build(clientContextWithDataStoreUpdateSink)
	if err != nil {
		return nil, err
	}
	client.store = store

	bigSegments := config.BigSegments
	if bigSegments == nil {
		bigSegments = ldcomponents.BigSegments(nil)
	}
	bsConfig, err := bigSegments.Build(clientContext)
	if err != nil {
		return nil, err
	}
	bsStore := bsConfig.GetStore()
	client.bigSegmentStoreStatusBroadcaster = internal.NewBroadcaster[interfaces.BigSegmentStoreStatus]()
	if bsStore != nil {
		client.bigSegmentStoreWrapper = ldstoreimpl.NewBigSegmentStoreWrapperWithConfig(
			ldstoreimpl.BigSegmentsConfigurationProperties{
				Store:              bsStore,
				StartPolling:       true,
				StatusPollInterval: bsConfig.GetStatusPollInterval(),
				StaleAfter:         bsConfig.GetStaleAfter(),
				ContextCacheSize:   bsConfig.GetContextCacheSize(),
				ContextCacheTime:   bsConfig.GetContextCacheTime(),
			},
			client.bigSegmentStoreStatusBroadcaster.Broadcast,
			loggers,
		)
		client.bigSegmentStoreStatusProvider = bigsegments.NewBigSegmentStoreStatusProviderImpl(
			client.bigSegmentStoreWrapper.GetStatus,
			client.bigSegmentStoreStatusBroadcaster,
		)
	} else {
		client.bigSegmentStoreStatusProvider = bigsegments.NewBigSegmentStoreStatusProviderImpl(
			nil, client.bigSegmentStoreStatusBroadcaster,
		)
	}

	dataProvider := ldstoreimpl.NewDataStoreEvaluatorDataProvider(store, loggers)
	evalOptions := []ldeval.EvaluatorOption{
		ldeval.EvaluatorOptionErrorLogger(client.loggers.ForLevel(ldlog.Error)),
	}
	if client.bigSegmentStoreWrapper != nil {
		evalOptions = append(evalOptions, ldeval.EvaluatorOptionBigSegmentProvider(client.bigSegmentStoreWrapper))
	}
	client.evaluator = ldeval.NewEvaluatorWithOptions(dataProvider, evalOptions...)

	client.dataStoreStatusProvider = datastore.NewDataStoreStatusProviderImpl(store, dataStoreUpdateSink)

	client.dataSourceStatusBroadcaster = internal.NewBroadcaster[interfaces.DataSourceStatus]()
	client.flagChangeEventBroadcaster = internal.NewBroadcaster[interfaces.FlagChangeEvent]()
	dataSourceUpdateSink := datasource.NewDataSourceUpdateSinkImpl(
		store,
		client.dataStoreStatusProvider,
		client.dataSourceStatusBroadcaster,
		client.flagChangeEventBroadcaster,
		clientContext.GetLogging().LogDataSourceOutageAsErrorAfter,
		loggers,
	)

	client.eventProcessor, err = eventProcessorFactory.Build(clientContext)
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

	dataSource, err := createDataSource(config, clientContext, dataSourceUpdateSink)
	client.dataSource = dataSource
	if err != nil {
		return nil, err
	}
	client.dataSourceStatusProvider = datasource.NewDataSourceStatusProviderImpl(
		client.dataSourceStatusBroadcaster,
		dataSourceUpdateSink,
	)

	client.flagTracker = internal.NewFlagTrackerImpl(
		client.flagChangeEventBroadcaster,
		func(flagKey string, context ldcontext.Context, defaultValue ldvalue.Value) ldvalue.Value {
			value, _ := client.JSONVariation(flagKey, context, defaultValue)
			return value
		},
	)

	client.hookRunner = hooks.NewRunner(loggers, config.Hooks)

	clientValid = true
	client.dataSource.Start(closeWhenReady)
	if waitFor > 0 && client.dataSource != datasource.NewNullDataSource() {
		loggers.Infof("Waiting up to %d milliseconds for LaunchDarkly client to start...",
			waitFor/time.Millisecond)

		// If you use a long duration and wait for the timeout, then any network delays will cause
		// your application to wait a long time before continuing execution.
		if waitFor.Seconds() > highWaitForSeconds {
			loggers.Warnf("Client was created was with a timeout greater than %d. "+
				"We recommend a timeout of less than %d seconds", highWaitForSeconds, highWaitForSeconds)
		}

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

func createDataSource(
	config Config,
	context *internal.ClientContextImpl,
	dataSourceUpdateSink subsystems.DataSourceUpdateSink,
) (subsystems.DataSource, error) {
	if config.Offline {
		context.GetLogging().Loggers.Info("Starting LaunchDarkly client in offline mode")
		dataSourceUpdateSink.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
		return datasource.NewNullDataSource(), nil
	}
	factory := config.DataSource
	if factory == nil {
		// COVERAGE: can't cause this condition in unit tests because it would try to connect to production LD
		factory = ldcomponents.StreamingDataSource()
	}
	contextCopy := *context
	contextCopy.BasicClientContext.DataSourceUpdateSink = dataSourceUpdateSink
	return factory.Build(&contextCopy)
}

// MigrationVariation returns the migration stage of the migration feature flag for the given evaluation context.
//
// Returns defaultStage if there is an error or if the flag doesn't exist.
func (client *LDClient) MigrationVariation(
	key string, context ldcontext.Context, defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return client.migrationVariation(gocontext.TODO(), key, context, defaultStage, client.eventsDefault,
		migrationVarFuncName)
}

// MigrationVariationCtx returns the migration stage of the migration feature flag for the given evaluation context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// Returns defaultStage if there is an error or if the flag doesn't exist.
func (client *LDClient) MigrationVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultStage ldmigration.Stage,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	return client.migrationVariation(ctx, key, context, defaultStage, client.eventsDefault, migrationVarExFuncName)
}

func (client *LDClient) migrationVariation(
	ctx gocontext.Context,
	key string, context ldcontext.Context, defaultStage ldmigration.Stage, eventsScope eventsScope, method string,
) (ldmigration.Stage, interfaces.LDMigrationOpTracker, error) {
	defaultStageAsValue := ldvalue.String(string(defaultStage))

	detail, flag, err := client.hookRunner.RunEvaluation(
		ctx,
		key,
		context,
		defaultStageAsValue,
		method,
		func() (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
			detail, flag, err := client.variationAndFlag(key, context, defaultStageAsValue, true,
				eventsScope)

			if err != nil {
				// Detail will already contain the default.
				// We do not have an error on the flag-not-found case.
				return detail, flag, nil
			}

			_, err = ldmigration.ParseStage(detail.Value.StringValue())
			if err != nil {
				detail = ldreason.NewEvaluationDetailForError(ldreason.EvalErrorWrongType, ldvalue.String(string(defaultStage)))
				return detail, flag, fmt.Errorf("%s; returning default stage %s", err, defaultStage)
			}

			return detail, flag, err
		},
	)

	tracker := NewMigrationOpTracker(key, flag, context, detail, defaultStage)
	// Stage will have already been parsed and defaulted.
	stage, _ := ldmigration.ParseStage(detail.Value.StringValue())

	return stage, tracker, err
}

// Identify reports details about an evaluation context.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/identify#go
func (client *LDClient) Identify(context ldcontext.Context) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if err := context.Err(); err != nil {
		client.loggers.Warnf("Identify called with invalid context: %s", err)
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}

	// Identify events should always sample
	evt := client.eventsDefault.factory.NewIdentifyEventData(ldevents.Context(context), ldvalue.NewOptionalInt(1))
	client.eventProcessor.RecordIdentifyEvent(evt)
	return nil
}

// TrackEvent reports an event associated with an evaluation context.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard. If you want to associate additional data with this event, use [TrackData]
// or [TrackMetric].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/events#go
func (client *LDClient) TrackEvent(eventName string, context ldcontext.Context) error {
	return client.TrackData(eventName, context, ldvalue.Null())
}

// TrackData reports an event associated with an evaluation context, and adds custom data.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the ldvalue.Value type, that
// will be sent with the event. If no such value is needed, use [ldvalue.Null]() (or call [TrackEvent]
// instead). To send a numeric value for experimentation, use [TrackMetric].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/events#go
func (client *LDClient) TrackData(eventName string, context ldcontext.Context, data ldvalue.Value) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if err := context.Err(); err != nil {
		client.loggers.Warnf("Track called with invalid context: %s", err)
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}

	client.eventProcessor.RecordCustomEvent(
		client.eventsDefault.factory.NewCustomEventData(
			eventName,
			ldevents.Context(context),
			data,
			false,
			0,
			ldvalue.NewOptionalInt(1),
		))
	return nil
}

// TrackMetric reports an event associated with an evaluation context, and adds a numeric value.
// This value is used by the LaunchDarkly experimentation feature in numeric custom metrics, and will also
// be returned as part of the custom event for Data Export.
//
// The eventName parameter is defined by the application and will be shown in analytics reports;
// it normally corresponds to the event name of a metric that you have created through the
// LaunchDarkly dashboard.
//
// The data parameter is a value of any JSON type, represented with the ldvalue.Value type, that
// will be sent with the event. If no such value is needed, use [ldvalue.Null]().
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/events#go
func (client *LDClient) TrackMetric(
	eventName string,
	context ldcontext.Context,
	metricValue float64,
	data ldvalue.Value,
) error {
	if client.eventsDefault.disabled {
		return nil
	}
	if err := context.Err(); err != nil {
		client.loggers.Warnf("TrackMetric called with invalid context: %s", err)
		return nil // Don't return an error value because we didn't in the past and it might confuse users
	}
	client.eventProcessor.RecordCustomEvent(
		client.eventsDefault.factory.NewCustomEventData(
			eventName,
			ldevents.Context(context),
			data,
			true,
			metricValue,
			ldvalue.NewOptionalInt(1),
		))
	return nil
}

// TrackMigrationOp reports a migration operation event.
func (client *LDClient) TrackMigrationOp(event ldevents.MigrationOpEventData) error {
	if client.eventsDefault.disabled {
		return nil
	}

	client.eventProcessor.RecordMigrationOpEvent(event)
	return nil
}

// IsOffline returns whether the LaunchDarkly client is in offline mode.
//
// This is only true if you explicitly set the Offline field to true in [Config], to force the client to
// be offline. It does not mean that the client is having a problem connecting to LaunchDarkly. To detect
// the status of a client that is configured to be online, use [LDClient.Initialized] or
// [LDClient.GetDataSourceStatusProvider].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/offline-mode#go
func (client *LDClient) IsOffline() bool {
	return client.offline
}

// SecureModeHash generates the secure mode hash value for an evaluation context.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/secure-mode#go
func (client *LDClient) SecureModeHash(context ldcontext.Context) string {
	key := []byte(client.sdkKey)
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(context.FullyQualifiedKey()))
	return hex.EncodeToString(h.Sum(nil))
}

// Initialized returns whether the LaunchDarkly client is initialized.
//
// If this value is true, it means the client has succeeded at some point in connecting to LaunchDarkly and
// has received feature flag data. It could still have encountered a connection problem after that point, so
// this does not guarantee that the flags are up to date; if you need to know its status in more detail, use
// [LDClient.GetDataSourceStatusProvider].
//
// If this value is false, it means the client has not yet connected to LaunchDarkly, or has permanently
// failed. See [MakeClient] for the reasons that this could happen. In this state, feature flag evaluations
// will always return default values-- unless you are using a database integration and feature flags had
// already been stored in the database by a successfully connected SDK in the past. You can use
// [LDClient.GetDataSourceStatusProvider] to get information on errors, or to wait for a successful retry.
func (client *LDClient) Initialized() bool {
	return client.dataSource.IsInitialized()
}

// Close shuts down the LaunchDarkly client. After calling this, the LaunchDarkly client
// should no longer be used. The method will block until all pending analytics events (if any)
// been sent.
func (client *LDClient) Close() error {
	client.loggers.Info("Closing LaunchDarkly client")

	// Normally all of the following components exist; but they could be nil if we errored out
	// partway through the MakeCustomClient constructor, in which case we want to close whatever
	// did get created so far.
	if client.eventProcessor != nil {
		_ = client.eventProcessor.Close()
	}
	if client.dataSource != nil {
		_ = client.dataSource.Close()
	}
	if client.store != nil {
		_ = client.store.Close()
	}
	if client.dataSourceStatusBroadcaster != nil {
		client.dataSourceStatusBroadcaster.Close()
	}
	if client.dataStoreStatusBroadcaster != nil {
		client.dataStoreStatusBroadcaster.Close()
	}
	if client.flagChangeEventBroadcaster != nil {
		client.flagChangeEventBroadcaster.Close()
	}
	if client.bigSegmentStoreStatusBroadcaster != nil {
		client.bigSegmentStoreStatusBroadcaster.Close()
	}
	if client.bigSegmentStoreWrapper != nil {
		client.bigSegmentStoreWrapper.Close()
	}
	return nil
}

// Flush tells the client that all pending analytics events (if any) should be delivered as soon
// as possible. This flush is asynchronous, so this method will return before it is complete. To wait
// for the flush to complete, use [LDClient.FlushAndWait] instead (or, if you are done with the SDK,
// [LDClient.Close]).
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/flush#go
func (client *LDClient) Flush() {
	client.eventProcessor.Flush()
}

// FlushAndWait tells the client to deliver any pending analytics events synchronously now.
//
// Unlike [LDClient.Flush], this method waits for event delivery to finish. The timeout parameter, if
// greater than zero, specifies the maximum amount of time to wait. If the timeout elapses before
// delivery is finished, the method returns early and returns false; in this case, the SDK may still
// continue trying to deliver the events in the background.
//
// If the timeout parameter is zero or negative, the method waits as long as necessary to deliver the
// events. However, the SDK does not retry event delivery indefinitely; currently, any network error
// or server error will cause the SDK to wait one second and retry one time, after which the events
// will be discarded so that the SDK will not keep consuming more memory for events indefinitely.
//
// The method returns true if event delivery either succeeded, or definitively failed, before the
// timeout elapsed. It returns false if the timeout elapsed.
//
// This method is also implicitly called if you call [LDClient.Close]. The difference is that
// FlushAndWait does not shut down the SDK client.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/flush#go
func (client *LDClient) FlushAndWait(timeout time.Duration) bool {
	return client.eventProcessor.FlushBlocking(timeout)
}

// Loggers exposes the logging component used by the SDK.
//
// This allows users to easily log messages to a shared channel with the SDK.
func (client *LDClient) Loggers() interfaces.LDLoggers {
	return client.loggers
}

// AllFlagsState returns an object that encapsulates the state of all feature flags for a given evaluation.
// context. This includes the flag values, and also metadata that can be used on the front end.
//
// The most common use case for this method is to bootstrap a set of client-side feature flags from a
// back-end service.
//
// You may pass any combination of [flagstate.ClientSideOnly], [flagstate.WithReasons], and
// [flagstate.DetailsOnlyForTrackedFlags] as optional parameters to control what data is included.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/all-flags#go
func (client *LDClient) AllFlagsState(context ldcontext.Context, options ...flagstate.Option) flagstate.AllFlags {
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

				result := client.evaluator.Evaluate(flag, context, nil)

				state.AddFlag(
					item.Key,
					flagstate.FlagState{
						Value:                result.Detail.Value,
						Variation:            result.Detail.VariationIndex,
						Reason:               result.Detail.Reason,
						Version:              flag.Version,
						TrackEvents:          flag.TrackEvents || result.IsExperiment,
						TrackReason:          result.IsExperiment,
						DebugEventsUntilDate: flag.DebugEventsUntilDate,
					},
				)
			}
		}
	}

	return state.Build()
}

// BoolVariation returns the value of a boolean feature flag for a given evaluation context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) BoolVariation(key string, context ldcontext.Context, defaultVal bool) (bool, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Bool(defaultVal), true,
		client.eventsDefault, boolVarFuncName)
	return detail.Value.BoolValue(), err
}

// BoolVariationDetail is the same as [LDClient.BoolVariation], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) BoolVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (bool, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Bool(defaultVal), true,
		client.eventsWithReasons, boolVarDetailFuncName)
	return detail.Value.BoolValue(), detail, err
}

// BoolVariationCtx is the same as [LDClient.BoolVariation], but accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) BoolVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (bool, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Bool(defaultVal), true,
		client.eventsDefault, boolVarExFuncName)
	return detail.Value.BoolValue(), err
}

// BoolVariationDetailCtx is the same as [LDClient.BoolVariationCtx], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) BoolVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal bool,
) (bool, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Bool(defaultVal), true,
		client.eventsWithReasons, boolVarDetailExFuncName)
	return detail.Value.BoolValue(), detail, err
}

// IntVariation returns the value of a feature flag (whose variations are integers) for the given evaluation
// context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// If the flag variation has a numeric value that is not an integer, it is rounded toward zero (truncated).
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) IntVariation(key string, context ldcontext.Context, defaultVal int) (int, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Int(defaultVal), true,
		client.eventsDefault, intVarFuncName)
	return detail.Value.IntValue(), err
}

// IntVariationDetail is the same as [LDClient.IntVariation], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) IntVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal int,
) (int, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Int(defaultVal), true,
		client.eventsWithReasons, intVarDetailFuncName)
	return detail.Value.IntValue(), detail, err
}

// IntVariationCtx is the same as [LDClient.IntVariation], but accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// If the flag variation has a numeric value that is not an integer, it is rounded toward zero (truncated).
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) IntVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal int,
) (int, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Int(defaultVal), true,
		client.eventsDefault, intVarExFuncName)
	return detail.Value.IntValue(), err
}

// IntVariationDetailCtx is the same as [LDClient.IntVariationCtx], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) IntVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal int,
) (int, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Int(defaultVal), true,
		client.eventsWithReasons, intVarDetailExFuncName)
	return detail.Value.IntValue(), detail, err
}

// Float64Variation returns the value of a feature flag (whose variations are floats) for the given evaluation
// context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) Float64Variation(key string, context ldcontext.Context, defaultVal float64) (float64, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Float64(defaultVal),
		true, client.eventsDefault, floatVarFuncName)
	return detail.Value.Float64Value(), err
}

// Float64VariationDetail is the same as [LDClient.Float64Variation], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) Float64VariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.Float64(defaultVal),
		true, client.eventsWithReasons, floatVarDetailFuncName)
	return detail.Value.Float64Value(), detail, err
}

// Float64VariationCtx is the same as [LDClient.Float64Variation], but accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and
// has no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) Float64VariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (float64, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Float64(defaultVal), true,
		client.eventsDefault, floatVarExFuncName)
	return detail.Value.Float64Value(), err
}

// Float64VariationDetailCtx is the same as [LDClient.Float64VariationCtx], but also returns further information about
// how the value was calculated. The "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) Float64VariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal float64,
) (float64, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.Float64(defaultVal), true,
		client.eventsWithReasons, floatVarDetailExFuncName)
	return detail.Value.Float64Value(), detail, err
}

// StringVariation returns the value of a feature flag (whose variations are strings) for the given evaluation
// context.
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and has
// no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) StringVariation(key string, context ldcontext.Context, defaultVal string) (string, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.String(defaultVal), true,
		client.eventsDefault, stringVarFuncName)
	return detail.Value.StringValue(), err
}

// StringVariationDetail is the same as [LDClient.StringVariation], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) StringVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, ldvalue.String(defaultVal), true,
		client.eventsWithReasons, stringVarDetailFuncName)
	return detail.Value.StringValue(), detail, err
}

// StringVariationCtx is the same as [LDClient.StringVariation], but accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off and has
// no off variation.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) StringVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal string,
) (string, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.String(defaultVal), true,
		client.eventsDefault, stringVarExFuncName)
	return detail.Value.StringValue(), err
}

// StringVariationDetailCtx is the same as [LDClient.StringVariationCtx], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) StringVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal string,
) (string, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, ldvalue.String(defaultVal), true,
		client.eventsWithReasons, stringVarDetailExFuncName)
	return detail.Value.StringValue(), detail, err
}

// JSONVariation returns the value of a feature flag for the given evaluation context, allowing the value to
// be of any JSON type.
//
// The value is returned as an [ldvalue.Value], which can be inspected or converted to other types using
// methods such as [ldvalue.Value.GetType] and [ldvalue.Value.BoolValue]. The defaultVal parameter also uses this
// type. For instance, if the values for this flag are JSON arrays:
//
//	defaultValAsArray := ldvalue.BuildArray().
//	    Add(ldvalue.String("defaultFirstItem")).
//	    Add(ldvalue.String("defaultSecondItem")).
//	    Build()
//	result, err := client.JSONVariation(flagKey, context, defaultValAsArray)
//	firstItemAsString := result.GetByIndex(0).StringValue() // "defaultFirstItem", etc.
//
// You can also use unparsed json.RawMessage values:
//
//	defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"things":[1,2,3]}`))
//	result, err := client.JSONVariation(flagKey, context, defaultValAsJSON
//	resultAsRawJSON := result.AsRaw()
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) JSONVariation(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (ldvalue.Value, error) {
	detail, _, err := client.variationWithHooks(gocontext.TODO(), key, context, defaultVal, false, client.eventsDefault,
		jsonVarFuncName)
	return detail.Value, err
}

// JSONVariationDetail is the same as [LDClient.JSONVariation], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) JSONVariationDetail(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(
		gocontext.TODO(),
		key,
		context,
		defaultVal,
		false,
		client.eventsWithReasons,
		jsonVarDetailFuncName,
	)
	return detail.Value, detail, err
}

// JSONVariationCtx is the same as [LDClient.JSONVariation], but accepts a context.Context.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// The value is returned as an [ldvalue.Value], which can be inspected or converted to other types using
// methods such as [ldvalue.Value.GetType] and [ldvalue.Value.BoolValue]. The defaultVal parameter also uses this
// type. For instance, if the values for this flag are JSON arrays:
//
//	defaultValAsArray := ldvalue.BuildArray().
//	    Add(ldvalue.String("defaultFirstItem")).
//	    Add(ldvalue.String("defaultSecondItem")).
//	    Build()
//	result, err := client.JSONVariationCtx(ctx, flagKey, context, defaultValAsArray)
//	firstItemAsString := result.GetByIndex(0).StringValue() // "defaultFirstItem", etc.
//
// You can also use unparsed json.RawMessage values:
//
//	defaultValAsRawJSON := ldvalue.Raw(json.RawMessage(`{"things":[1,2,3]}`))
//	result, err := client.JSONVariation(flagKey, context, defaultValAsJSON
//	resultAsRawJSON := result.AsRaw()
//
// Returns defaultVal if there is an error, if the flag doesn't exist, or the feature is turned off.
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluating#go
func (client *LDClient) JSONVariationCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (ldvalue.Value, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, defaultVal, false, client.eventsDefault,
		jsonVarExFuncName)
	return detail.Value, err
}

// JSONVariationDetailCtx is the same as [LDClient.JSONVariationCtx], but also returns further information about how
// the value was calculated. The "reason" data will also be included in analytics events.
//
// Cancelling the context.Context will not cause the evaluation to be cancelled. The context.Context is used
// by hook implementations refer to [ldhooks.Hook].
//
// For more information, see the Reference Guide: https://docs.launchdarkly.com/sdk/features/evaluation-reasons#go
func (client *LDClient) JSONVariationDetailCtx(
	ctx gocontext.Context,
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
) (ldvalue.Value, ldreason.EvaluationDetail, error) {
	detail, _, err := client.variationWithHooks(ctx, key, context, defaultVal, false, client.eventsWithReasons,
		jsonVarDetailExFuncName)
	return detail.Value, detail, err
}

// GetDataSourceStatusProvider returns an interface for tracking the status of the data source.
//
// The data source is the mechanism that the SDK uses to get feature flag configurations, such as a
// streaming connection (the default) or poll requests. The [interfaces.DataSourceStatusProvider] has methods
// for checking whether the data source is (as far as the SDK knows) currently operational and tracking
// changes in this status.
//
// See the DataSourceStatusProvider interface for more about this functionality.
func (client *LDClient) GetDataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	return client.dataSourceStatusProvider
}

// GetDataStoreStatusProvider returns an interface for tracking the status of a persistent data store.
//
// The [interfaces.DataStoreStatusProvider] has methods for checking whether the data store is (as far as the SDK
// knows) currently operational, tracking changes in this status, and getting cache statistics. These
// are only relevant for a persistent data store; if you are using an in-memory data store, then this
// method will always report that the store is operational.
//
// See the DataStoreStatusProvider interface for more about this functionality.
func (client *LDClient) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return client.dataStoreStatusProvider
}

// GetFlagTracker returns an interface for tracking changes in feature flag configurations.
//
// See [interfaces.FlagTracker] for more about this functionality.
func (client *LDClient) GetFlagTracker() interfaces.FlagTracker {
	return client.flagTracker
}

// GetBigSegmentStoreStatusProvider returns an interface for tracking the status of a Big
// Segment store.
//
// The BigSegmentStoreStatusProvider has methods for checking whether the Big Segment store
// is (as far as the SDK knows) currently operational and tracking changes in this status.
//
// See [interfaces.BigSegmentStoreStatusProvider] for more about this functionality.
func (client *LDClient) GetBigSegmentStoreStatusProvider() interfaces.BigSegmentStoreStatusProvider {
	return client.bigSegmentStoreStatusProvider
}

// WithEventsDisabled returns a decorator for the LDClient that implements the same basic operations
// but will not generate any analytics events.
//
// If events were already disabled, this is just the same LDClient. Otherwise, it is an object whose
// Variation methods use the same LDClient to evaluate feature flags, but without generating any
// events, and whose Identify/Track/Custom methods do nothing. Neither evaluation counts nor context
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

func (client *LDClient) variationWithHooks(
	context gocontext.Context,
	key string,
	evalContext ldcontext.Context,
	defaultVal ldvalue.Value,
	checkType bool,
	eventsScope eventsScope,
	method string,
) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
	detail, flag, err := client.hookRunner.RunEvaluation(
		context,
		key,
		evalContext,
		defaultVal,
		method,
		func() (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
			return client.variationAndFlag(key, evalContext, defaultVal, checkType, eventsScope)
		},
	)
	return detail, flag, err
}

// Generic method for evaluating a feature flag for a given evaluation context,
// returning both the result and the flag.
func (client *LDClient) variationAndFlag(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
	checkType bool,
	eventsScope eventsScope,
) (ldreason.EvaluationDetail, *ldmodel.FeatureFlag, error) {
	if err := context.Err(); err != nil {
		client.loggers.Warnf("Tried to evaluate a flag with an invalid context: %s", err)
		return newEvaluationError(defaultVal, ldreason.EvalErrorUserNotSpecified), nil, err
	}
	if client.IsOffline() {
		return newEvaluationError(defaultVal, ldreason.EvalErrorClientNotReady), nil, nil
	}
	result, flag, err := client.evaluateInternal(key, context, defaultVal, eventsScope)
	if err != nil {
		result.Detail.Value = defaultVal
		result.Detail.VariationIndex = ldvalue.OptionalInt{}
	} else if checkType && defaultVal.Type() != ldvalue.NullType && result.Detail.Value.Type() != defaultVal.Type() {
		result.Detail = newEvaluationError(defaultVal, ldreason.EvalErrorWrongType)
	}

	if !eventsScope.disabled {
		var eval ldevents.EvaluationData
		if flag == nil {
			eval = eventsScope.factory.NewUnknownFlagEvaluationData(
				key,
				ldevents.Context(context),
				defaultVal,
				result.Detail.Reason,
			)
		} else {
			eval = eventsScope.factory.NewEvaluationData(
				ldevents.FlagEventProperties{
					Key:                  flag.Key,
					Version:              flag.Version,
					RequireFullEvent:     flag.TrackEvents,
					DebugEventsUntilDate: flag.DebugEventsUntilDate,
				},
				ldevents.Context(context),
				result.Detail,
				result.IsExperiment,
				defaultVal,
				"",
				flag.SamplingRatio,
				flag.ExcludeFromSummaries,
			)
		}
		client.eventProcessor.RecordEvaluation(eval)
	}

	return result.Detail, flag, err
}

// Performs all the steps of evaluation except for sending the feature request event (the main one;
// events for prerequisites will be sent).
func (client *LDClient) evaluateInternal(
	key string,
	context ldcontext.Context,
	defaultVal ldvalue.Value,
	eventsScope eventsScope,
) (ldeval.Result, *ldmodel.FeatureFlag, error) {
	// THIS IS A HIGH-TRAFFIC CODE PATH so performance tuning is important. Please see CONTRIBUTING.md for guidelines
	// to keep in mind during any changes to the evaluation logic.

	var feature *ldmodel.FeatureFlag
	var storeErr error
	var ok bool

	evalErrorResult := func(
		errKind ldreason.EvalErrorKind,
		flag *ldmodel.FeatureFlag,
		err error,
	) (ldeval.Result, *ldmodel.FeatureFlag, error) {
		detail := newEvaluationError(defaultVal, errKind)
		if client.logEvaluationErrors {
			client.loggers.Warn(err)
		}
		return ldeval.Result{Detail: detail}, flag, err
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
		return ldeval.Result{Detail: detail}, nil, storeErr
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

	result := client.evaluator.Evaluate(feature, context, eventsScope.prerequisiteEventRecorder)
	if result.Detail.Reason.GetKind() == ldreason.EvalReasonError && client.logEvaluationErrors {
		client.loggers.Warnf("Flag evaluation for %s failed with error %s, default value was returned",
			key, result.Detail.Reason.GetErrorKind())
	}
	if result.Detail.IsDefaultValue() {
		result.Detail.Value = defaultVal
	}
	return result, feature, nil
}

func newEvaluationError(jsonValue ldvalue.Value, errorKind ldreason.EvalErrorKind) ldreason.EvaluationDetail {
	return ldreason.EvaluationDetail{
		Value:  jsonValue,
		Reason: ldreason.NewEvalReasonError(errorKind),
	}
}
