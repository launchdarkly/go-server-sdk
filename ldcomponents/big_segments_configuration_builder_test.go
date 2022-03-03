package ldcomponents

import (
	"errors"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBigSegmentStoreFactory struct {
	fakeError error
}

func (m mockBigSegmentStoreFactory) CreateBigSegmentStore(interfaces.ClientContext) (interfaces.BigSegmentStore, error) {
	return mockBigSegmentStore{}, m.fakeError
}

type mockBigSegmentStore struct{}

func (m mockBigSegmentStore) Close() error { return nil }

func (m mockBigSegmentStore) GetMetadata() (interfaces.BigSegmentStoreMetadata, error) {
	return interfaces.BigSegmentStoreMetadata{}, nil
}

func (m mockBigSegmentStore) GetUserMembership(string) (interfaces.BigSegmentMembership, error) {
	return nil, nil
}

func TestBigSegmentsConfigurationBuilder(t *testing.T) {
	context := basicClientContext()

	t.Run("defaults", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).CreateBigSegmentsConfiguration(context)
		require.NoError(t, err)

		assert.Equal(t, mockBigSegmentStore{}, c.GetStore())
		assert.Equal(t, DefaultBigSegmentsUserCacheSize, c.GetUserCacheSize())
		assert.Equal(t, DefaultBigSegmentsUserCacheTime, c.GetUserCacheTime())
		assert.Equal(t, DefaultBigSegmentsStatusPollInterval, c.GetStatusPollInterval())
		assert.Equal(t, DefaultBigSegmentsStaleAfter, c.GetStaleAfter())
	})

	t.Run("store creation fails", func(t *testing.T) {
		fakeError := errors.New("sorry")
		storeFactory := mockBigSegmentStoreFactory{fakeError: fakeError}
		_, err := BigSegments(storeFactory).CreateBigSegmentsConfiguration(context)
		require.Equal(t, fakeError, err)
	})

	t.Run("UserCacheSize", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			UserCacheSize(999).
			CreateBigSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, 999, c.GetUserCacheSize())
	})

	t.Run("UserCacheTime", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			UserCacheTime(time.Second * 999).
			CreateBigSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetUserCacheTime())
	})

	t.Run("StatusPollInterval", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			StatusPollInterval(time.Second * 999).
			CreateBigSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStatusPollInterval())
	})

	t.Run("StaleAfter", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			StaleAfter(time.Second * 999).
			CreateBigSegmentsConfiguration(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStaleAfter())
	})
}
