package mocks

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
)

// MockStatusReporter is a mock implementation of DataSourceUpdates for testing data sources.
type MockStatusReporter struct {
	Statuses   chan interfaces.DataSourceStatus
	lastStatus interfaces.DataSourceStatus
	lock       sync.Mutex
}

// NewMockStatusReporter creates an instance of MockStatusReporter.
//
// The DataStoreStatusProvider can be nil if we are not doing a test that requires manipulation of that
// component.
func NewMockStatusReporter() *MockStatusReporter {
	return &MockStatusReporter{
		Statuses: make(chan interfaces.DataSourceStatus, 10),
	}
}

// UpdateStatus in this test implementation, pushes a value onto the Statuses channel.
func (d *MockStatusReporter) UpdateStatus(
	newState interfaces.DataSourceState,
	newError interfaces.DataSourceErrorInfo,
) {
	d.lock.Lock()
	defer d.lock.Unlock()
	if newState != d.lastStatus.State || newError.Kind != "" {
		d.lastStatus = interfaces.DataSourceStatus{State: newState, LastError: newError}
		d.Statuses <- d.lastStatus
	}
}

// RequireStatusOf blocks until a new data source status is available, and verifies its state.
func (d *MockStatusReporter) RequireStatusOf(
	t *testing.T,
	newState interfaces.DataSourceState,
) interfaces.DataSourceStatus {
	status := d.RequireStatus(t)
	assert.Equal(t, string(newState), string(status.State))
	// string conversion is due to a bug in assert with type aliases
	return status
}

// RequireStatus blocks until a new data source status is available.
func (d *MockStatusReporter) RequireStatus(t *testing.T) interfaces.DataSourceStatus {
	return th.RequireValue(t, d.Statuses, time.Second, "timed out waiting for new data source status")
}
