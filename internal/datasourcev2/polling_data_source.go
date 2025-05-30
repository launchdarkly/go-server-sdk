package datasourcev2

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
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
	Request(context.Context, subsystems.Selector) (*subsystems.ChangeSet, error)
	BaseURI() string
	FilterKey() string
}

// PollingProcessor is the internal implementation of the polling data source.
//
// This type is exported from internal so that the PollingDataSourceBuilder tests can verify its
// configuration. All other code outside of this package should interact with it only via the
// DataSource interface.
type PollingProcessor struct {
	requester    PollingRequester
	pollInterval time.Duration
	loggers      ldlog.Loggers
	isClosed     atomic.Bool
	quit         chan struct{}
}

// NewPollingProcessor creates the internal implementation of the polling data source.
func NewPollingProcessor(
	context subsystems.ClientContext,
	cfg datasource.PollingConfig,
) *PollingProcessor {
	httpRequester := newPollingRequester(context, context.GetHTTP().CreateHTTPClient(), cfg.BaseURI, cfg.FilterKey)
	return newPollingProcessor(context, httpRequester, cfg.PollInterval)
}

func newPollingProcessor(
	context subsystems.ClientContext,
	requester PollingRequester,
	pollInterval time.Duration,
) *PollingProcessor {
	pp := &PollingProcessor{
		requester:    requester,
		pollInterval: pollInterval,
		loggers:      context.GetLogging().Loggers,
		quit:         make(chan struct{}),
	}
	return pp
}

//nolint:revive // DataInitializer method.
func (pp *PollingProcessor) Name() string {
	return "PollingDataSourceV2"
}

//nolint:revive // DataInitializer method.
func (pp *PollingProcessor) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, error) {
	changeSet, err := pp.requester.Request(ctx, ds.Selector())
	if err != nil {
		return nil, err
	}
	return &subsystems.Basis{ChangeSet: *changeSet, Persist: true}, nil
}

//nolint:revive // DataSynchronizer method.
func (pp *PollingProcessor) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	resultChan := make(chan subsystems.DataSynchronizerResult)
	pp.loggers.Infof("Starting LaunchDarkly polling with interval: %+v", pp.pollInterval)

	if pp.isClosed.Load() {
		pp.loggers.Warnf("Polling processor is already closed, not starting polling")
		close(resultChan)
		return resultChan
	}

	// This process has a shared method serving both as an initializer and a synchronizer.
	//
	// The initializers currently provide a cancellable context throughout
	// their call stack. Once we have done the same with the synchronizers, we
	// can the TODO context with a real one.
	ctx := context.TODO()

	ticker := newTickerWithInitialTick(pp.pollInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-pp.quit:
				close(resultChan)
				return
			case <-ticker.C:
				if err := pp.poll(ctx, ds, resultChan); err != nil {
					if hse, ok := err.(httpStatusError); ok {
						errorInfo := interfaces.DataSourceErrorInfo{
							Kind:       interfaces.DataSourceErrorKindErrorResponse,
							StatusCode: hse.Code,
							Time:       time.Now(),
						}

						if hse.Header.Get("X-LD-FD-Fallback") == "true" {
							resultChan <- subsystems.DataSynchronizerResult{
								State:        interfaces.DataSourceStateOff,
								Error:        errorInfo,
								RevertToFDv1: true,
							}
							return
						}

						recoverable := checkIfErrorIsRecoverableAndLog(
							pp.loggers,
							httpErrorDescription(hse.Code),
							pollingErrorContext,
							hse.Code,
							pollingWillRetryMessage,
						)
						if recoverable {
							resultChan <- subsystems.DataSynchronizerResult{
								State: interfaces.DataSourceStateInterrupted,
								Error: errorInfo,
							}
						} else {
							resultChan <- subsystems.DataSynchronizerResult{
								State: interfaces.DataSourceStateOff,
								Error: errorInfo,
							}
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
						resultChan <- subsystems.DataSynchronizerResult{
							State: interfaces.DataSourceStateInterrupted,
							Error: errorInfo,
						}
					}
					continue
				}
			}
		}
	}()

	return resultChan
}

func (pp *PollingProcessor) poll(
	ctx context.Context, ds subsystems.DataSelector, resultChan chan<- subsystems.DataSynchronizerResult,
) error {
	changeSet, err := pp.requester.Request(ctx, ds.Selector())
	if err != nil {
		return err
	}

	resultChan <- subsystems.DataSynchronizerResult{
		ChangeSet: changeSet,
		State:     interfaces.DataSourceStateValid,
	}

	return nil
}

//nolint:revive // no doc comment for standard method
func (pp *PollingProcessor) Close() error {
	if swapped := pp.isClosed.CompareAndSwap(false, true); swapped {
		close(pp.quit)
	}
	return nil
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
