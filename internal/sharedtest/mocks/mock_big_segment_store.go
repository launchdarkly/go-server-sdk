package mocks

import (
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/require"
)

// MockBigSegmentStore is a minimal mock implementation of BigSegmentStore. Currently it only
// supports specifying the metadata and simulating an error for metadata queries.
type MockBigSegmentStore struct {
	metadata          subsystems.BigSegmentStoreMetadata
	metadataErr       error
	metadataQueries   chan struct{}
	memberships       map[string]subsystems.BigSegmentMembership
	membershipQueries []string
	membershipErr     error
	lock              sync.Mutex
}

func (m *MockBigSegmentStore) Close() error { //nolint:revive
	return nil
}

func (m *MockBigSegmentStore) GetMetadata() (subsystems.BigSegmentStoreMetadata, error) { //nolint:revive
	m.lock.Lock()
	md, err := m.metadata, m.metadataErr
	if m.metadataQueries != nil {
		m.metadataQueries <- struct{}{}
	}
	m.lock.Unlock()
	return md, err
}

func (m *MockBigSegmentStore) TestSetMetadataState( //nolint:revive
	md subsystems.BigSegmentStoreMetadata,
	err error,
) {
	m.lock.Lock()
	m.metadata, m.metadataErr = md, err
	m.lock.Unlock()
}

func (m *MockBigSegmentStore) TestSetMetadataToCurrentTime() { //nolint:revive
	m.TestSetMetadataState(subsystems.BigSegmentStoreMetadata{LastUpToDate: ldtime.UnixMillisNow()}, nil)
}

func (m *MockBigSegmentStore) TestGetMetadataQueriesCh() <-chan struct{} { //nolint:revive
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.metadataQueries == nil {
		m.metadataQueries = make(chan struct{})
	}
	return m.metadataQueries
}

func (m *MockBigSegmentStore) GetMembership( //nolint:revive
	contextHash string,
) (subsystems.BigSegmentMembership, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipQueries = append(m.membershipQueries, contextHash)
	if m.membershipErr != nil {
		return nil, m.membershipErr
	}
	return m.memberships[contextHash], nil
}

func (m *MockBigSegmentStore) TestSetMembership( //nolint:revive
	contextHash string,
	membership subsystems.BigSegmentMembership,
) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.memberships == nil {
		m.memberships = make(map[string]subsystems.BigSegmentMembership)
	}
	m.memberships[contextHash] = membership
}

func (m *MockBigSegmentStore) TestSetMembershipError(err error) { //nolint:revive
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipErr = err
}

func (m *MockBigSegmentStore) TestGetMembershipQueries() []string { //nolint:revive
	m.lock.Lock()
	defer m.lock.Unlock()
	return append([]string(nil), m.membershipQueries...)
}

// ExpectBigSegmentStoreStatus waits for a status value to appear in a channel and also verifies that it
// matches the status currently being reported by the status provider.
func ExpectBigSegmentStoreStatus(
	t *testing.T,
	statusCh <-chan interfaces.BigSegmentStoreStatus,
	statusGetter func() interfaces.BigSegmentStoreStatus,
	timeout time.Duration,
	expectedStatus interfaces.BigSegmentStoreStatus,
) {
	newStatus := th.RequireValue(t, statusCh, timeout, "timed out waiting for new status")
	require.Equal(t, expectedStatus, newStatus)
	if statusGetter != nil {
		require.Equal(t, newStatus, statusGetter())
	}
}
