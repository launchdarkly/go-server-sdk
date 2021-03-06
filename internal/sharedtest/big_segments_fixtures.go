package sharedtest

import (
	"sync"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldtime"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/require"
)

// SingleBigSegmentStoreFactory is an BigSegmentStoreFactory that returns an existing instance.
type SingleBigSegmentStoreFactory struct {
	Store *MockBigSegmentStore
}

func (f SingleBigSegmentStoreFactory) CreateBigSegmentStore(interfaces.ClientContext) ( //nolint:golint
	interfaces.BigSegmentStore, error) {
	return f.Store, nil
}

// MockBigSegmentStore is a minimal mock implementation of BigSegmentStore. Currently it only
// supports specifying the metadata and simulating an error for metadata queries.
type MockBigSegmentStore struct {
	metadata          interfaces.BigSegmentStoreMetadata
	metadataErr       error
	memberships       map[string]interfaces.BigSegmentMembership
	membershipQueries []string
	membershipErr     error
	lock              sync.Mutex
}

func (m *MockBigSegmentStore) Close() error { //nolint:golint
	return nil
}

func (m *MockBigSegmentStore) GetMetadata() (interfaces.BigSegmentStoreMetadata, error) { //nolint:golint
	m.lock.Lock()
	md, err := m.metadata, m.metadataErr
	m.lock.Unlock()
	return md, err
}

func (m *MockBigSegmentStore) TestSetMetadataState( //nolint:golint
	md interfaces.BigSegmentStoreMetadata,
	err error,
) {
	m.lock.Lock()
	m.metadata, m.metadataErr = md, err
	m.lock.Unlock()
}

func (m *MockBigSegmentStore) TestSetMetadataToCurrentTime() { //nolint:golint
	m.TestSetMetadataState(interfaces.BigSegmentStoreMetadata{LastUpToDate: ldtime.UnixMillisNow()}, nil)
}

func (m *MockBigSegmentStore) GetUserMembership( //nolint:golint
	userHash string,
) (interfaces.BigSegmentMembership, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipQueries = append(m.membershipQueries, userHash)
	if m.membershipErr != nil {
		return nil, m.membershipErr
	}
	return m.memberships[userHash], nil
}

func (m *MockBigSegmentStore) TestSetMembership( //nolint:golint
	userHash string,
	membership interfaces.BigSegmentMembership,
) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.memberships == nil {
		m.memberships = make(map[string]interfaces.BigSegmentMembership)
	}
	m.memberships[userHash] = membership
}

func (m *MockBigSegmentStore) TestSetMembershipError(err error) { //nolint:golint
	m.lock.Lock()
	defer m.lock.Unlock()
	m.membershipErr = err
}

func (m *MockBigSegmentStore) TestGetMembershipQueries() []string { //nolint:golint
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
