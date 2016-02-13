package ldclient

import (
	"sync"
	"time"
)

type pollingProcessor struct {
	store              FeatureStore
	requestor          *requestor
	config             Config
	setInitializedOnce sync.Once
	isInitialized      bool
	lastHeaders        *cacheHeaders
}

func newPollingProcessor(config Config, store FeatureStore, requestor *requestor) updateProcessor {
	pp := &pollingProcessor{
		store:     store,
		requestor: requestor,
		config:    config,
	}

	return pp
}

func (pp *pollingProcessor) start(ch chan<- bool) {
	go func() {
		for {
			then := time.Now()
			err := pp.poll()
			if err == nil {
				pp.setInitializedOnce.Do(func() {
					pp.isInitialized = true
					ch <- true
				})
			}
			delta := (1 * time.Second) - time.Since(then)

			if delta > 0 {
				time.Sleep(delta)
			}
		}
	}()
}

func (pp *pollingProcessor) poll() error {
	features, nextHdrs, err := pp.requestor.makeAllRequest(pp.lastHeaders, true)

	if err != nil {
		return err
	}

	// We get nextHdrs only if we got a 200 response, which means we need to
	// update the store. Otherwise we'll have gotten a 304 (do nothing) or an
	// error
	if nextHdrs != nil {
		return pp.store.Init(features)
	}
	return nil
}

// TODO add support for canceling the goroutine
func (pp *pollingProcessor) close() {

}

func (pp *pollingProcessor) initialized() bool {
	return pp.isInitialized
}
