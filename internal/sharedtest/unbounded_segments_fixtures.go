package sharedtest

import (
	"sync"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/require"
)

// SingleUnboundedSegmentStoreFactory is an UnboundedSegmentStoreFactory that returns an existing instance.
type SingleUnboundedSegmentStoreFactory struct {
	Store *MockUnboundedSegmentStore
}

func (f SingleUnboundedSegmentStoreFactory) CreateUnboundedSegmentStore(interfaces.ClientContext) ( //nolint:golint
	interfaces.UnboundedSegmentStore, error) {
	return f.Store, nil
}

// MockUnboundedSegmentStore is a minimal mock implementation of UnboundedSegmentStore. Currently it only
// supports specifying the metadata and simulating an error for metadata queries.
type MockUnboundedSegmentStore struct {
	metadata          interfaces.UnboundedSegmentStoreMetadata
	metadataErr       error
	memberships       map[string]interfaces.UnboundedSegmentMembership
	membershipQueries []string
	membershipErr     error
	lock              sync.Mutex
}

func (m *MockUnboundedSegmentStore) Close() error { //nolint:golint
	return nil
}

func (m *MockUnboundedSegmentStore) GetMetadata() (interfaces.UnboundedSegmentStoreMetadata, error) { //nolint:golint
	m.lock.Lock()
	md, err := m.metadata, m.metadataErr
	m.lock.Unlock()
	return md, err
}

func (m *MockUnboundedSegmentStore) TestSetMetadataState( //nolint:golint
	md interfaces.UnboundedSegmentStoreMetadata,
	err error,
) {
	m.lock.Lock()
	m.metadata, m.metadataErr = md, err
	m.lock.Unlock()
}

func (m *MockUnboundedSegmentStore) TestSetMetadataToCurrentTime() { //nolint:golint
	m.TestSetMetadataState(interfaces.UnboundedSegmentStoreMetadata{LastUpToDate: ldtime.UnixMillisNow()}, nil)
}

func (m *MockUnboundedSegmentStore) GetUserMembership( //nolint:golint
	userHash string,
) (interfaces.UnboundedSegmentMembership, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipQueries = append(m.membershipQueries, userHash)
	if m.membershipErr != nil {
		return nil, m.membershipErr
	}
	return m.memberships[userHash], nil
}

func (m *MockUnboundedSegmentStore) TestSetMembership( //nolint:golint
	userHash string,
	membership interfaces.UnboundedSegmentMembership,
) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.memberships == nil {
		m.memberships = make(map[string]interfaces.UnboundedSegmentMembership)
	}
	m.memberships[userHash] = membership
}

func (m *MockUnboundedSegmentStore) TestSetMembershipError(err error) { //nolint:golint
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipErr = err
}

func (m *MockUnboundedSegmentStore) TestGetMembershipQueries() []string { //nolint:golint
	m.lock.Lock()
	defer m.lock.Unlock()
	return append([]string(nil), m.membershipQueries...)
}

// ExpectUnboundedSegmentStoreStatus waits for a status value to appear in a channel and also verifies that it
// matches the status currently being reported by the status provider.
func ExpectUnboundedSegmentStoreStatus(
	t *testing.T,
	statusCh <-chan interfaces.UnboundedSegmentStoreStatus,
	statusGetter func() interfaces.UnboundedSegmentStoreStatus,
	timeout time.Duration,
	expectedStatus interfaces.UnboundedSegmentStoreStatus,
) {
	select {
	case newStatus := <-statusCh:
		require.Equal(t, expectedStatus, newStatus)
		if statusGetter != nil {
			require.Equal(t, newStatus, statusGetter())
		}
	case <-time.After(timeout):
		require.Fail(t, "timed out waiting for new status")
	}
}
