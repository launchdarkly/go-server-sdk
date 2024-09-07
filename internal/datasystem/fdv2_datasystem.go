package datasystem

import (
	"context"
	"errors"
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datastore"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"sync"
)

type store struct {
	persistentStore     subsystems.DataStore
	persistentStoreMode subsystems.StoreMode

	memoryStore subsystems.DataStore
	memory      bool
	refreshed   bool
	mu          sync.RWMutex
}

func newStore(persistent subsystems.DataStore, mode subsystems.StoreMode, loggers ldlog.Loggers) *store {
	return &store{
		persistentStore:     persistent,
		persistentStoreMode: mode,
		memoryStore:         datastore.NewInMemoryDataStore(loggers),
	}
}

func (s *store) Close() error {
	if s.persistentStore != nil {
		return s.persistentStore.Close()
	}
	return nil
}

func (s *store) GetActive() subsystems.DataStore {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.memory {
		return s.memoryStore
	}
	return s.persistentStore
}

func (s *store) Status() DataStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// The logic here is:
	// 1. If the memory store is active, we either got that data from an (initializer|synchronizer) that indicated
	// the data is the latest known (Refreshed) or that it is potentially stale (Cached). This is set when SwapToMemory
	// is called.
	// 2. Otherwise, the persistent store - if any - is active. If there is none configured, the status is Defaults.
	//   If there is, we need to query the database availability to determine if we actually have access to the data
	//   or not.
	if s.memory {
		if s.refreshed {
			return Refreshed
		}
		return Cached
	}
	if s.persistentStore != nil {
		if s.persistentStore.IsInitialized() {
			return Cached
		}
	}
	return Defaults

}

func (s *store) GetMemory() subsystems.DataStore {
	return s.memoryStore
}

func (s *store) SwapToMemory(isRefreshed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.memory = true
	s.refreshed = isRefreshed
}

type FDv2 struct {
	store *store

	initializers  []subsystems.DataInitializer
	primarySync   subsystems.DataSynchronizer
	secondarySync subsystems.DataSynchronizer

	offline bool

	loggers ldlog.Loggers

	cancel context.CancelFunc
	done   chan struct{}

	readyOnce sync.Once
}

func NewFDv2(cfgBuilder subsystems.ComponentConfigurer[subsystems.DataSystemConfiguration], clientContext *internal.ClientContextImpl) (*FDv2, error) {
	cfg, err := cfgBuilder.Build(*clientContext)
	if err != nil {
		return nil, err
	}
	return &FDv2{
		store:         newStore(cfg.Store, cfg.StoreMode, clientContext.GetLogging().Loggers),
		initializers:  cfg.Initializers,
		primarySync:   cfg.Synchronizers.Primary,
		secondarySync: cfg.Synchronizers.Secondary,
		offline:       cfg.Offline,
		loggers:       clientContext.GetLogging().Loggers,
		done:          make(chan struct{}),
	}, nil
}

func (f *FDv2) Start(closeWhenReady chan struct{}) {
	ctx, cancel := context.WithCancel(context.Background())
	f.cancel = cancel
	go func() {
		defer close(f.done)
		payloadVersion := f.runInitializers(ctx, closeWhenReady)
		f.runSynchronizers(ctx, closeWhenReady, payloadVersion)
	}()
}

func (f *FDv2) runInitializers(ctx context.Context, closeWhenReady chan struct{}) *int {
	for _, initializer := range f.initializers {
		payload, err := initializer.Fetch(ctx)
		if errors.Is(err, context.Canceled) {
			return nil
		}
		if err != nil {
			continue
		}
		_ = f.store.GetMemory().Init(payload.Data)
		f.store.SwapToMemory(payload.Fresh)
		f.readyOnce.Do(func() {
			close(closeWhenReady)
		})
		return payload.Version
	}
	return nil
}

func (f *FDv2) runSynchronizers(ctx context.Context, closeWhenReady chan struct{}, payloadVersion *int) {
	// If the SDK was configured with no synchronizer, then (assuming no initializer succeeded), we should
	// trigger the ready signal to let the call to MakeClient unblock immediately.
	if f.primarySync == nil {
		f.readyOnce.Do(func() {
			close(closeWhenReady)
		})
		return
	}

	ready := make(chan struct{})
	f.primarySync.Start(ready, f.store.GetMemory(), payloadVersion)

	for {
		select {
		case <-ready:
			// We may have synchronizers that don't actually validate that a payload is fresh. In this case,
			// we'd need a mechanism to propagate the status to this method, just like for the initializers.
			// For now, we assume that the only synchronizers are LaunchDarkly-provided and do receive fresh payloads.
			f.store.SwapToMemory(true)
			f.readyOnce.Do(func() {
				close(closeWhenReady)
			})
		case <-ctx.Done():
			return
		}
	}
}

func (f *FDv2) Stop() error {
	if f.cancel != nil {
		f.cancel()
		<-f.done
	}
	_ = f.store.Close()
	if f.primarySync != nil {
		_ = f.primarySync.Close()
	}
	if f.secondarySync != nil {
		_ = f.secondarySync.Close()
	}
	return nil
}

func (f *FDv2) Store() subsystems.ReadOnlyStore {
	return f.store.GetActive()
}

func (f *FDv2) DataStatus() DataStatus {
	if f.offline {
		return Defaults
	}
	return f.store.Status()
}

func (f *FDv2) DataSourceStatusBroadcaster() *internal.Broadcaster[interfaces.DataSourceStatus] {
	//TODO implement me
	panic("implement me")
}

func (f *FDv2) DataSourceStatusProvider() interfaces.DataSourceStatusProvider {
	//TODO implement me
	panic("implement me")
}

func (f *FDv2) DataStoreStatusBroadcaster() *internal.Broadcaster[interfaces.DataStoreStatus] {
	//TODO implement me
	panic("implement me")
}

func (f *FDv2) DataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	//TODO implement me
	panic("implement me")
}

func (f *FDv2) FlagChangeEventBroadcaster() *internal.Broadcaster[interfaces.FlagChangeEvent] {
	//TODO implement me
	panic("implement me")
}

func (f *FDv2) Offline() bool {
	return f.offline
}
