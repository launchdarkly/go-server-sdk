package ldcomponents

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockUnboundedSegmentStoreFactory struct {
	fakeError error
}

func (m mockUnboundedSegmentStoreFactory) CreateUnboundedSegmentStore(interfaces.ClientContext) (interfaces.UnboundedSegmentStore, error) {
	return mockUnboundedSegmentStore{}, m.fakeError
}

type mockUnboundedSegmentStore struct{}

func (m mockUnboundedSegmentStore) Close() error { return nil }

func (m mockUnboundedSegmentStore) GetMetadata() (interfaces.UnboundedSegmentStoreMetadata, error) {
	return interfaces.UnboundedSegmentStoreMetadata{}, nil
}

func (m mockUnboundedSegmentStore) GetUserMembership(string) (interfaces.UnboundedSegmentMembership, error) {
	return nil, nil
}

func TestUnboundedSegmentsConfigurationBuilder(t *testing.T) {
	context := basicClientContext()

	t.Run("defaults", func(t *testing.T) {
		c, err := UnboundedSegments(mockUnboundedSegmentStoreFactory{}).CreateUnboundedSegmentsConfiguration(context)
		require.NoError(t, err)

		assert.Equal(t, mockUnboundedSegmentStore{}, c.GetStore())
		assert.Equal(t, DefaultUnboundedSegmentsUserCacheSize, c.GetUserCacheSize())
		assert.Equal(t, DefaultUnboundedSegmentsUserCacheTime, c.GetUserCacheTime())
		assert.Equal(t, DefaultUnboundedSegmentsStatusPollInterval, c.GetStatusPollInterval())
		assert.Equal(t, DefaultUnboundedSegmentsStaleAfter, c.GetStaleAfter())
	})

	t.Run("store creation fails", func(t *testing.T) {
		fakeError := errors.New("sorry")
		storeFactory := mockUnboundedSegmentStoreFactory{fakeError: fakeError}
		_, err := UnboundedSegments(storeFactory).CreateUnboundedSegmentsConfiguration(context)
		require.Equal(t, fakeError, err)
	})

	t.Run("UserCacheSize", func(t *testing.T) {
		c, err := UnboundedSegments(mockUnboundedSegmentStoreFactory{}).
			UserCacheSize(999).
			CreateUnboundedSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, 999, c.GetUserCacheSize())
	})

	t.Run("UserCacheTime", func(t *testing.T) {
		c, err := UnboundedSegments(mockUnboundedSegmentStoreFactory{}).
			UserCacheTime(time.Second * 999).
			CreateUnboundedSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetUserCacheTime())
	})

	t.Run("StatusPollInterval", func(t *testing.T) {
		c, err := UnboundedSegments(mockUnboundedSegmentStoreFactory{}).
			StatusPollInterval(time.Second * 999).
			CreateUnboundedSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStatusPollInterval())
	})

	t.Run("StaleAfter", func(t *testing.T) {
		c, err := UnboundedSegments(mockUnboundedSegmentStoreFactory{}).
			StaleAfter(time.Second * 999).
			CreateUnboundedSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStaleAfter())
	})
}
