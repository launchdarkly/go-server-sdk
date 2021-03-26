package bigsegments

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlogtest"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"
)

type storeManagerTestParams struct {
	t             *testing.T
	store         *sharedtest.MockBigSegmentStore
	manager       *BigSegmentStoreManager
	pollInterval  time.Duration
	staleTime     time.Duration
	userCacheSize int
	userCacheTime time.Duration
	mockLog       *ldlogtest.MockLog
}

func storeManagerTest(t *testing.T) *storeManagerTestParams {
	return &storeManagerTestParams{
		t:             t,
		store:         &sharedtest.MockBigSegmentStore{},
		pollInterval:  time.Millisecond * 10,
		staleTime:     time.Hour,
		userCacheSize: 1000,
		userCacheTime: time.Hour,
		mockLog:       ldlogtest.NewMockLog(),
	}
}

func (p *storeManagerTestParams) run(action func(*storeManagerTestParams)) {
	defer p.mockLog.DumpIfTestFailed(p.t)
	p.manager = NewBigSegmentStoreManager(p.store, p.pollInterval, p.staleTime,
		p.userCacheSize, p.userCacheTime, p.mockLog.Loggers)
	p.store.TestSetMetadataToCurrentTime()
	defer p.manager.Close()
	action(p)
}

func (p *storeManagerTestParams) assertMembership(userKey string, expected interfaces.BigSegmentMembership) {
	membership, ok := p.manager.getUserMembership(userKey)
	assert.True(p.t, ok)
	assert.Equal(p.t, expected, membership)
}

func (p *storeManagerTestParams) assertUserHashesQueried(hashes ...string) {
	assert.Equal(p.t, hashes, p.store.TestGetMembershipQueries())
}
