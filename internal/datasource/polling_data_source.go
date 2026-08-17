package datasource

import (
	"net/http"
	"sync"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

const (
	pollingErrorContext     = "on polling request"
	pollingWillRetryMessage = "will retry at next scheduled poll interval"
)

// PollingConfig describes the configuration for a polling data source. It is exported so that
// it can be used in the PollingDataSourceBuilder.
type PollingConfig struct {
	BaseURI                     string
	PollInterval                time.Duration
	FilterKey                   string
	ExtendedInitialPollInterval time.Duration
}

// Requester allows PollingProcessor to delegate fetching data to another component.
// This is useful for testing the PollingProcessor without needing to set up a test HTTP server.
type Requester interface {
	Request() (data []ldstoretypes.Collection, cached bool, headers http.Header, err error)
	BaseURI() string
	FilterKey() string
}

// PollingProcessor is the internal implementation of the polling data source.
//
// This type is exported from internal so that the PollingDataSourceBuilder tests can verify its
// configuration. All other code outside of this package should interact with it only via the
// DataSource interface.
type PollingProcessor struct {
	dataSourceUpdates  subsystems.DataSourceUpdateSink
	requester          Requester
	pollInterval       time.Duration
	strategy           *pollingStrategy
	loggers            ldlog.Loggers
	setInitializedOnce sync.Once
	isInitialized      internal.AtomicBoolean
	quit               chan struct{}
	closeOnce          sync.Once
}

// NewPollingProcessor creates the internal implementation of the polling data source.
func NewPollingProcessor(
	context subsystems.ClientContext,
	dataSourceUpdates subsystems.DataSourceUpdateSink,
	cfg PollingConfig,
) *PollingProcessor {
	httpRequester := NewPollingRequester(context, context.GetHTTP().CreateHTTPClient(), cfg.BaseURI, cfg.FilterKey)
	return newPollingProcessor(
		context, dataSourceUpdates, httpRequester,
		cfg.PollInterval, cfg.ExtendedInitialPollInterval,
	)
}

func newPollingProcessor(
	context subsystems.ClientContext,
	dataSourceUpdates subsystems.DataSourceUpdateSink,
	requester Requester,
	pollInterval time.Duration,
	extendedInitialPollInterval time.Duration,
) *PollingProcessor {
	pp := &PollingProcessor{
		dataSourceUpdates: dataSourceUpdates,
		requester:         requester,
		pollInterval:      pollInterval,
		strategy:          newPollingStrategy(pollInterval, extendedInitialPollInterval),
		loggers:           context.GetLogging().Loggers,
		quit:              make(chan struct{}),
	}
	return pp
}

//nolint:revive // no doc comment for standard method
func (pp *PollingProcessor) Start(closeWhenReady chan<- struct{}) {
	pp.loggers.Infof("Starting LaunchDarkly polling with interval: %+v", pp.pollInterval)

	// Fires immediately for the first poll; Reset after each iteration to schedule
	// the next. Under RETRY (SDK-2775), the interval between polls is dynamic per
	// the pollingStrategy state machine, so we can't use a fixed-period Ticker.
	timer := time.NewTimer(0)

	go func() {
		defer timer.Stop()

		var readyOnce sync.Once
		notifyReady := func() {
			readyOnce.Do(func() {
				close(closeWhenReady)
			})
		}
		// Ensure we stop waiting for initialization if we exit, even if initialization fails
		defer notifyReady()

		for {
			select {
			case <-pp.quit:
				return
			case <-timer.C:
				if err := pp.poll(); err != nil {
					var class FailureClass
					if hse, ok := err.(httpStatusError); ok {
						errorInfo := interfaces.DataSourceErrorInfo{
							Kind:       interfaces.DataSourceErrorKindErrorResponse,
							StatusCode: hse.Code,
							Time:       time.Now(),
						}
						class = classifyAndLogHTTPFailure(
							pp.loggers,
							httpErrorDescription(hse.Code),
							pollingErrorContext,
							hse.Code,
							pollingWillRetryMessage,
						)
						pp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
					} else {
						errorInfo := interfaces.DataSourceErrorInfo{
							Kind:    interfaces.DataSourceErrorKindNetworkError,
							Message: err.Error(),
							Time:    time.Now(),
						}
						if _, ok := err.(malformedJSONError); ok {
							errorInfo.Kind = interfaces.DataSourceErrorKindInvalidData
						}
						class = classifyAndLogTransportFailure(
							pp.loggers, err, pollingErrorContext, pollingWillRetryMessage,
						)
						pp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
					}
					if pp.strategy.OnFailure(class) {
						pp.loggers.Info("Classified failure as UNEXPECTED; engaging extended backoff.")
					}
				} else {
					pp.dataSourceUpdates.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
					pp.setInitializedOnce.Do(func() {
						pp.isInitialized.Set(true)
						pp.loggers.Info("First polling request successful")
						notifyReady()
					})
					pp.strategy.OnSuccess()
				}
				timer.Reset(pp.strategy.NextWait())
			}
		}
	}()
}

func (pp *PollingProcessor) poll() error {
	allData, cached, headers, err := pp.requester.Request()

	if err != nil {
		return err
	}

	// We initialize the store only if the request wasn't cached
	if !cached {
		pp.dataSourceUpdates.Init(allData)
		if dataSourceWithInitMetadata, ok := pp.dataSourceUpdates.(subsystems.DataSourceUpdateSinkWithEnvironmentID); ok {
			initMetadata := internal.NewInitMetadataFromHeaders(headers)
			dataSourceWithInitMetadata.SetEnvironmentID(initMetadata.GetEnvironmentID())
		}
	}
	return nil
}

//nolint:revive // no doc comment for standard method
func (pp *PollingProcessor) Close() error {
	pp.closeOnce.Do(func() {
		close(pp.quit)
	})
	return nil
}

//nolint:revive // no doc comment for standard method
func (pp *PollingProcessor) IsInitialized() bool {
	return pp.isInitialized.Get()
}

// GetBaseURI returns the configured polling base URI, for testing.
func (pp *PollingProcessor) GetBaseURI() string {
	return pp.requester.BaseURI()
}

// GetPollInterval returns the configured polling interval, for testing.
func (pp *PollingProcessor) GetPollInterval() time.Duration {
	return pp.pollInterval
}

// GetFilterKey returns the configured filter key, for testing.
func (pp *PollingProcessor) GetFilterKey() string {
	return pp.requester.FilterKey()
}
