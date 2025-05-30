package mocks

import (
	"sync"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
)

// MockDataSelector is a mock implementation of the DataSelector interface.
type MockDataSelector struct {
	mutex    sync.Mutex
	selector subsystems.Selector
}

// NewMockDataSelector creates a new MockDataSelector with the given selector.
func NewMockDataSelector(selector subsystems.Selector) *MockDataSelector {
	return &MockDataSelector{
		selector: selector,
	}
}

// Selector returns the selector that this mock data selector holds.
func (m *MockDataSelector) Selector() subsystems.Selector {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	return m.selector
}

// SetSelector safely updates the selector.
func (m *MockDataSelector) SetSelector(selector subsystems.Selector) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.selector = selector
}
