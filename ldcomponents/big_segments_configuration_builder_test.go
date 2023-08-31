package ldcomponents

import (
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBigSegmentStoreFactory struct {
	fakeError error
}

func (m mockBigSegmentStoreFactory) Build(subsystems.ClientContext) (subsystems.BigSegmentStore, error) {
	return mockBigSegmentStore{}, m.fakeError
}

type mockBigSegmentStore struct{}

func (m mockBigSegmentStore) Close() error { return nil }

func (m mockBigSegmentStore) GetMetadata() (subsystems.BigSegmentStoreMetadata, error) {
	return subsystems.BigSegmentStoreMetadata{}, nil
}

func (m mockBigSegmentStore) GetMembership(string) (subsystems.BigSegmentMembership, error) {
	return nil, nil
}

func TestBigSegmentsConfigurationBuilder(t *testing.T) {
	context := basicClientContext()

	t.Run("defaults", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).Build(context)
		require.NoError(t, err)

		assert.Equal(t, mockBigSegmentStore{}, c.GetStore())
		assert.Equal(t, DefaultBigSegmentsContextCacheSize, c.GetContextCacheSize())
		assert.Equal(t, DefaultBigSegmentsContextCacheTime, c.GetContextCacheTime())
		assert.Equal(t, DefaultBigSegmentsStatusPollInterval, c.GetStatusPollInterval())
		assert.Equal(t, DefaultBigSegmentsStaleAfter, c.GetStaleAfter())
	})

	t.Run("store creation fails", func(t *testing.T) {
		fakeError := errors.New("sorry")
		storeFactory := mockBigSegmentStoreFactory{fakeError: fakeError}
		_, err := BigSegments(storeFactory).Build(context)
		require.Equal(t, fakeError, err)
	})

	t.Run("ContextCacheSize", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			ContextCacheSize(999).
			Build(context)
		require.NoError(t, err)
		assert.Equal(t, 999, c.GetContextCacheSize())
	})

	t.Run("ContextCacheTime", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			ContextCacheTime(time.Second * 999).
			Build(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetContextCacheTime())
	})

	t.Run("StatusPollInterval", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			StatusPollInterval(time.Second * 999).
			Build(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStatusPollInterval())
	})

	t.Run("StaleAfter", func(t *testing.T) {
		c, err := BigSegments(mockBigSegmentStoreFactory{}).
			StaleAfter(time.Second * 999).
			Build(context)
		require.NoError(t, err)
		assert.Equal(t, time.Second*999, c.GetStaleAfter())
	})
}
