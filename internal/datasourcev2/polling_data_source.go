package datasourcev2

import (
	"sync"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/internal/fdv2proto"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

const (
	pollingErrorContext     = "on polling request"
	pollingWillRetryMessage = "will retry at next scheduled poll interval"
)

// PollingRequester allows PollingProcessor to delegate fetching data to another component.
// This is useful for testing the PollingProcessor without needing to set up a test HTTP server.
type PollingRequester interface {
	Request() (*PollingResponse, error)
	BaseURI() string
	FilterKey() string
}

// PollingProcessor is the internal implementation of the polling data source.
//
// This type is exported from internal so that the PollingDataSourceBuilder tests can verify its
// configuration. All other code outside of this package should interact with it only via the
// DataSource interface.
type PollingProcessor struct {
	dataDestination    subsystems.DataDestination
	statusReporter     subsystems.DataSourceStatusReporter
	requester          PollingRequester
	pollInterval       time.Duration
	loggers            ldlog.Loggers
	setInitializedOnce sync.Once
	isInitialized      internal.AtomicBoolean
	quit               chan struct{}
	closeOnce          sync.Once
}

// NewPollingProcessor creates the internal implementation of the polling data source.
func NewPollingProcessor(
	context subsystems.ClientContext,
	dataDestination subsystems.DataDestination,
	statusReporter subsystems.DataSourceStatusReporter,
	cfg datasource.PollingConfig,
) *PollingProcessor {
	httpRequester := newPollingRequester(context, context.GetHTTP().CreateHTTPClient(), cfg.BaseURI, cfg.FilterKey)
	return newPollingProcessor(context, dataDestination, statusReporter, httpRequester, cfg.PollInterval)
}

func newPollingProcessor(
	context subsystems.ClientContext,
	dataDestination subsystems.DataDestination,
	statusReporter subsystems.DataSourceStatusReporter,
	requester PollingRequester,
	pollInterval time.Duration,
) *PollingProcessor {
	pp := &PollingProcessor{
		dataDestination: dataDestination,
		statusReporter:  statusReporter,
		requester:       requester,
		pollInterval:    pollInterval,
		loggers:         context.GetLogging().Loggers,
		quit:            make(chan struct{}),
	}
	return pp
}

//nolint:revive // no doc comment for standard method
func (pp *PollingProcessor) Start(closeWhenReady chan<- struct{}) {
	pp.loggers.Infof("Starting LaunchDarkly polling with interval: %+v", pp.pollInterval)

	ticker := newTickerWithInitialTick(pp.pollInterval)

	go func() {
		defer ticker.Stop()

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
			case <-ticker.C:
				if err := pp.poll(); err != nil {
					if hse, ok := err.(httpStatusError); ok {
						errorInfo := interfaces.DataSourceErrorInfo{
							Kind:       interfaces.DataSourceErrorKindErrorResponse,
							StatusCode: hse.Code,
							Time:       time.Now(),
						}
						recoverable := checkIfErrorIsRecoverableAndLog(
							pp.loggers,
							httpErrorDescription(hse.Code),
							pollingErrorContext,
							hse.Code,
							pollingWillRetryMessage,
						)
						if recoverable {
							pp.statusReporter.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
						} else {
							pp.statusReporter.UpdateStatus(interfaces.DataSourceStateOff, errorInfo)
							notifyReady()
							return
						}
					} else {
						errorInfo := interfaces.DataSourceErrorInfo{
							Kind:    interfaces.DataSourceErrorKindNetworkError,
							Message: err.Error(),
							Time:    time.Now(),
						}
						if _, ok := err.(malformedJSONError); ok {
							errorInfo.Kind = interfaces.DataSourceErrorKindInvalidData
						}
						checkIfErrorIsRecoverableAndLog(pp.loggers, err.Error(), pollingErrorContext, 0, pollingWillRetryMessage)
						pp.statusReporter.UpdateStatus(interfaces.DataSourceStateInterrupted, errorInfo)
					}
					continue
				}
				pp.statusReporter.UpdateStatus(interfaces.DataSourceStateValid, interfaces.DataSourceErrorInfo{})
				pp.setInitializedOnce.Do(func() {
					pp.isInitialized.Set(true)
					pp.loggers.Info("First polling request successful")
					notifyReady()
				})
			}
		}
	}()
}

func (pp *PollingProcessor) poll() error {
	response, err := pp.requester.Request()

	if err != nil {
		return err
	}

	if response.Cached() {
		return nil
	}

	//nolint:godox
	// TODO(SDK-712): If the destination fails to apply the updates should it affect the processor?
	// This would only happen in the case of a persistent store.
	switch response.Intent() {
	case fdv2proto.IntentTransferFull:
		_ = pp.dataDestination.SetBasis(response.Events(), response.Selector(), true)
	case fdv2proto.IntentTransferChanges:
		_ = pp.dataDestination.ApplyDelta(response.Events(), response.Selector(), true)
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

type tickerWithInitialTick struct {
	*time.Ticker
	C <-chan time.Time
}

func newTickerWithInitialTick(interval time.Duration) *tickerWithInitialTick {
	c := make(chan time.Time)
	ticker := time.NewTicker(interval)
	t := &tickerWithInitialTick{
		C:      c,
		Ticker: ticker,
	}
	go func() {
		c <- time.Now() // Ensure we do an initial poll immediately
		for tt := range ticker.C {
			c <- tt
		}
	}()
	return t
}
