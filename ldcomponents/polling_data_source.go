package ldcomponents

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

type pollingProcessor struct {
	store              interfaces.DataStore
	requestor          *requestor
	pollInterval       time.Duration
	loggers            ldlog.Loggers
	setInitializedOnce sync.Once
	isInitialized      bool
	quit               chan struct{}
	closeOnce          sync.Once
}

func newPollingProcessor(context interfaces.ClientContext, store interfaces.DataStore, requestor *requestor, pollInterval time.Duration) *pollingProcessor {
	pp := &pollingProcessor{
		store:        store,
		requestor:    requestor,
		pollInterval: pollInterval,
		loggers:      context.GetLoggers(),
		quit:         make(chan struct{}),
	}

	return pp
}

func (pp *pollingProcessor) Start(closeWhenReady chan<- struct{}) {
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
				pp.loggers.Info("Polling has been shut down")
				return
			case <-ticker.C:
				if err := pp.poll(); err != nil {
					pp.loggers.Errorf("Error when requesting feature updates: %+v", err)
					if hse, ok := err.(httpStatusError); ok {
						pp.loggers.Error(httpErrorMessage(hse.Code, "polling request", "will retry"))
						if !isHTTPErrorRecoverable(hse.Code) {
							notifyReady()
							return
						}
					}
					continue
				}
				pp.setInitializedOnce.Do(func() {
					pp.isInitialized = true
					pp.loggers.Info("First polling request successful")
					notifyReady()
				})
			}
		}
	}()
}

func (pp *pollingProcessor) poll() error {
	allData, cached, err := pp.requestor.requestAll()

	if err != nil {
		return err
	}

	// We initialize the store only if the request wasn't cached
	if !cached {
		return pp.store.Init(makeAllVersionedDataMap(allData.Flags, allData.Segments))
	}
	return nil
}

func (pp *pollingProcessor) Close() error {
	pp.closeOnce.Do(func() {
		close(pp.quit)
	})
	return nil
}

func (pp *pollingProcessor) Initialized() bool {
	return pp.isInitialized
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
