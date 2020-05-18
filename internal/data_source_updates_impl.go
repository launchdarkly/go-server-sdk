package internal

import (
	"sync"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	intf "gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// DataSourceUpdatesImpl is the internal implementation of DataSourceUpdates. It is exported
// because the actual implementation type, rather than the interface, is required as a dependency
// of other SDK components.
type DataSourceUpdatesImpl struct {
	store                   intf.DataStore
	dataStoreStatusProvider intf.DataStoreStatusProvider
	broadcaster             *DataSourceStatusBroadcaster
	loggers                 ldlog.Loggers
	currentStatus           intf.DataSourceStatus
	lastStoreUpdateFailed   bool
	lock                    sync.Mutex
}

// NewDataSourceUpdatesImpl creates the internal implementation of DataSourceUpdates.
func NewDataSourceUpdatesImpl(
	store intf.DataStore,
	dataStoreStatusProvider intf.DataStoreStatusProvider,
	broadcaster *DataSourceStatusBroadcaster,
	loggers ldlog.Loggers,
) *DataSourceUpdatesImpl {
	return &DataSourceUpdatesImpl{
		store:                   store,
		dataStoreStatusProvider: dataStoreStatusProvider,
		broadcaster:             broadcaster,
		loggers:                 loggers,
		currentStatus: intf.DataSourceStatus{
			State:      intf.DataSourceStateInitializing,
			StateSince: time.Now(),
		},
	}
}

// Init is a standard method of DataSourceUpdates.
func (d *DataSourceUpdatesImpl) Init(allData []intf.StoreCollection) bool {
	err := d.store.Init(sortCollectionsForDataStoreInit(allData))
	return d.maybeUpdateError(err)
}

// Upsert is a standard method of DataSourceUpdates.
func (d *DataSourceUpdatesImpl) Upsert(
	kind intf.StoreDataKind,
	key string,
	item intf.StoreItemDescriptor,
) bool {
	err := d.store.Upsert(kind, key, item)
	return d.maybeUpdateError(err)
}

func (d *DataSourceUpdatesImpl) maybeUpdateError(err error) bool {
	if err == nil {
		d.lock.Lock()
		defer d.lock.Unlock()
		d.lastStoreUpdateFailed = false
		return true
	}

	d.UpdateStatus(
		intf.DataSourceStateInterrupted,
		intf.DataSourceErrorInfo{
			Kind:    intf.DataSourceErrorKindStoreError,
			Message: err.Error(),
			Time:    time.Now(),
		},
	)

	shouldLog := false
	d.lock.Lock()
	shouldLog = !d.lastStoreUpdateFailed
	d.lastStoreUpdateFailed = true
	d.lock.Unlock()
	if shouldLog {
		d.loggers.Warnf("Unexpected data store error when trying to store an update received from the data source: %s", err)
	}

	return false
}

// UpdateStatus is a standard method of DataSourceUpdates.
func (d *DataSourceUpdatesImpl) UpdateStatus(
	newState intf.DataSourceState,
	newError intf.DataSourceErrorInfo,
) {
	if newState == "" {
		return
	}
	if statusToBroadcast, changed := d.maybeUpdateStatus(newState, newError); changed {
		d.broadcaster.Broadcast(statusToBroadcast)
	}
}

func (d *DataSourceUpdatesImpl) maybeUpdateStatus(
	newState intf.DataSourceState,
	newError intf.DataSourceErrorInfo,
) (intf.DataSourceStatus, bool) {
	d.lock.Lock()
	defer d.lock.Unlock()

	oldStatus := d.currentStatus

	if newState == intf.DataSourceStateInterrupted && oldStatus.State == intf.DataSourceStateInitializing {
		newState = intf.DataSourceStateInitializing // see comment on DataSourceUpdates.UpdateStatus
	}

	if newState == oldStatus.State && newError.Kind == "" {
		return intf.DataSourceStatus{}, false
	}

	stateSince := oldStatus.StateSince
	if newState != oldStatus.State {
		stateSince = time.Now()
	}
	lastError := oldStatus.LastError
	if newError.Kind != "" {
		lastError = newError
	}
	d.currentStatus = intf.DataSourceStatus{
		State:      newState,
		StateSince: stateSince,
		LastError:  lastError,
	}
	return d.currentStatus, true
}

// GetDataStoreStatusProvider is a standard method of DataSourceUpdates.
func (d *DataSourceUpdatesImpl) GetDataStoreStatusProvider() intf.DataStoreStatusProvider {
	return d.dataStoreStatusProvider
}

// GetLastStatus is used internally by SDK components.
func (d *DataSourceUpdatesImpl) GetLastStatus() intf.DataSourceStatus {
	d.lock.Lock()
	defer d.lock.Unlock()
	return d.currentStatus
}

func (d *DataSourceUpdatesImpl) waitFor(desiredState intf.DataSourceState, timeout time.Duration) bool {
	d.lock.Lock()
	if d.currentStatus.State == desiredState {
		d.lock.Unlock()
		return true
	}
	if d.currentStatus.State == intf.DataSourceStateOff {
		d.lock.Unlock()
		return false
	}

	statusCh := d.broadcaster.AddListener()
	defer d.broadcaster.RemoveListener(statusCh)
	d.lock.Unlock()

	var deadline <-chan time.Time
	if timeout > 0 {
		deadline = time.After(timeout)
	}

	for {
		select {
		case newStatus := <-statusCh:
			if newStatus.State == desiredState {
				return true
			}
			if newStatus.State == intf.DataSourceStateOff {
				return false
			}
		case <-deadline:
			return false
		}
	}
}
