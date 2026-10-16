package datasourcev2

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datasource"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

const (
	pollingErrorContext     = "on polling request"
	pollingWillRetryMessage = "will retry at next scheduled poll interval"

	// Consecutive successful polls that clear the extended backoff.
	pollResetThreshold = 2
)

// PollingRequester allows PollingProcessor to delegate fetching data to another component.
// This is useful for testing the PollingProcessor without needing to set up a test HTTP server.
type PollingRequester interface {
	Request(context.Context, subsystems.Selector) (*subsystems.ChangeSet, http.Header, error)
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
func (pp *PollingProcessor) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error) {
	changeSet, headers, err := pp.requester.Request(ctx, ds.Selector())
	fallback := isFDv1FallbackRequested(headers)
	if err != nil {
		return nil, fallback, err
	}
	environmentID := internal.NewInitMetadataFromHeaders(headers).GetEnvironmentID()
	return &subsystems.Basis{ChangeSet: *changeSet, Persist: true, EnvironmentID: environmentID}, fallback, nil
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

		// send delivers a result and reports whether it did. The consumer stops reading when it
		// closes the processor, so send gives up on Close instead of blocking this goroutine forever.
		send := func(result subsystems.DataSynchronizerResult) bool {
			select {
			case resultChan <- result:
				return true
			case <-pp.quit:
				close(resultChan)
				return false
			}
		}
		consecutiveSuccesses := 0

		for {
			select {
			case <-pp.quit:
				close(resultChan)
				return
			case <-ticker.C:
				result, err := pp.poll(ctx, ds)

				if err == nil {
					consecutiveSuccesses++
					if consecutiveSuccesses == pollResetThreshold {
						result.ResetExtendedBackoff = true
					}
				} else {
					consecutiveSuccesses = 0
				}

				// When the server requested FDv1 fallback, dispatch the result as-is -- poll has
				// already populated State (Valid on success, Interrupted on error) and FallbackToFDv1=true.
				if result.FallbackToFDv1 {
					send(result)
					return
				}

				if err == nil {
					if !send(result) {
						return
					}
					continue
				}

				// Non-fallback error: mark it an unexpected failure when it is not recoverable.
				// Log at the appropriate level.
				if hse, ok := err.(httpStatusError); ok {
					if checkIfErrorIsRecoverableAndLog(
						pp.loggers,
						httpErrorDescription(hse.Code),
						pollingErrorContext,
						hse.Code,
						pollingWillRetryMessage,
					) {
						result.State = interfaces.DataSourceStateInterrupted
						if !send(result) {
							return
						}
						continue
					}
					result.UnexpectedFailure = true
					send(result) // poll set State=Off
					return
				}

				checkIfErrorIsRecoverableAndLog(pp.loggers, err.Error(), pollingErrorContext, 0, pollingWillRetryMessage)
				result.State = interfaces.DataSourceStateInterrupted
				if !send(result) {
					return
				}
			}
		}
	}()

	return resultChan
}

// poll performs a single polling request and builds a DataSynchronizerResult describing the
// outcome. The result's FallbackToFDv1 flag is always populated from the x-ld-fd-fallback response
// header, whether or not the request succeeded -- a 500 or a malformed-JSON body can still carry
// the fallback signal.
//
// On success: result.State = Valid, result.ChangeSet populated, err = nil.
// On error: result.State = Interrupted, result.Error populated with Kind/Message/StatusCode as
// appropriate, err returned so the caller can apply context-specific logic (e.g. mark the result
// an unexpected failure when the HTTP error is not recoverable).
//
// The caller is responsible for publishing the result to its channel; poll does not touch any
// resultChan so it can be unit-tested in isolation.
func (pp *PollingProcessor) poll(
	ctx context.Context, ds subsystems.DataSelector,
) (subsystems.DataSynchronizerResult, error) {
	changeSet, headers, err := pp.requester.Request(ctx, ds.Selector())
	result := subsystems.DataSynchronizerResult{
		EnvironmentID:  internal.NewInitMetadataFromHeaders(headers).GetEnvironmentID(),
		FallbackToFDv1: isFDv1FallbackRequested(headers),
	}

	if err == nil {
		result.ChangeSet = changeSet
		result.State = interfaces.DataSourceStateValid
		return result, nil
	}

	result.State = interfaces.DataSourceStateInterrupted
	if hse, ok := err.(httpStatusError); ok {
		result.Error = interfaces.DataSourceErrorInfo{
			Kind:       interfaces.DataSourceErrorKindErrorResponse,
			StatusCode: hse.Code,
			Time:       time.Now(),
		}
	} else {
		result.Error = interfaces.DataSourceErrorInfo{
			Kind:    interfaces.DataSourceErrorKindNetworkError,
			Message: err.Error(),
			Time:    time.Now(),
		}
		if _, ok := err.(malformedJSONError); ok {
			result.Error.Kind = interfaces.DataSourceErrorKindInvalidData
		}
	}
	return result, err
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

// tickerWithInitialTick is like a time.Ticker, but it also ticks once as soon as it is created.
type tickerWithInitialTick struct {
	ticker *time.Ticker
	C      <-chan time.Time
	stop   chan struct{}
}

func newTickerWithInitialTick(interval time.Duration) *tickerWithInitialTick {
	c := make(chan time.Time)
	t := &tickerWithInitialTick{
		ticker: time.NewTicker(interval),
		C:      c,
		stop:   make(chan struct{}),
	}
	go func() {
		tick := time.Now() // Ensure we do an initial poll immediately
		for {
			select {
			case c <- tick:
			case <-t.stop:
				return
			}
			select {
			case tick = <-t.ticker.C:
			case <-t.stop:
				return
			}
		}
	}()
	return t
}

// Stop stops the ticker and ends the goroutine that forwards its ticks. Stopping a time.Ticker
// does not close its channel, so that goroutine cannot rely on the channel to end. Call Stop
// only once.
func (t *tickerWithInitialTick) Stop() {
	t.ticker.Stop()
	close(t.stop)
}
