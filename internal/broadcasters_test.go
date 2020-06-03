package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

func TestDataStoreStatusBroadcaster(t *testing.T) {
	status1 := interfaces.DataStoreStatus{Available: true}

	t.Run("broadcast with no subscribers", func(t *testing.T) {
		b := NewDataStoreStatusBroadcaster()
		defer b.Close()

		b.Broadcast(status1)
	})

	t.Run("broadcast with subscribers", func(t *testing.T) {
		b := NewDataStoreStatusBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()

		b.Broadcast(status1)

		assert.Equal(t, status1, <-ch1)
		assert.Equal(t, status1, <-ch2)
	})

	t.Run("unregister subscriber", func(t *testing.T) {
		b := NewDataStoreStatusBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()
		b.RemoveListener(ch1)

		b.Broadcast(status1)

		assert.Len(t, ch1, 0)
		assert.Equal(t, status1, <-ch2)
	})
}

func TestDataSourceStatusBroadcaster(t *testing.T) {
	status1 := interfaces.DataSourceStatus{State: interfaces.DataSourceStateValid}

	t.Run("broadcast with no subscribers", func(t *testing.T) {
		b := NewDataSourceStatusBroadcaster()
		defer b.Close()

		b.Broadcast(status1)
	})

	t.Run("broadcast with subscribers", func(t *testing.T) {
		b := NewDataSourceStatusBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()

		b.Broadcast(status1)

		assert.Equal(t, status1, <-ch1)
		assert.Equal(t, status1, <-ch2)
	})

	t.Run("unregister subscriber", func(t *testing.T) {
		b := NewDataSourceStatusBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()
		b.RemoveListener(ch1)

		b.Broadcast(status1)

		assert.Len(t, ch1, 0)
		assert.Equal(t, status1, <-ch2)
	})
}

func TestFlagChangeEventBroadcaster(t *testing.T) {
	event := interfaces.FlagChangeEvent{Key: "flag"}

	t.Run("broadcast with no subscribers", func(t *testing.T) {
		b := NewFlagChangeEventBroadcaster()
		defer b.Close()

		b.Broadcast(event)
	})

	t.Run("broadcast with subscribers", func(t *testing.T) {
		b := NewFlagChangeEventBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()

		b.Broadcast(event)

		assert.Equal(t, event, <-ch1)
		assert.Equal(t, event, <-ch2)
	})

	t.Run("unregister subscriber", func(t *testing.T) {
		b := NewFlagChangeEventBroadcaster()
		defer b.Close()

		ch1 := b.AddListener()
		ch2 := b.AddListener()
		b.RemoveListener(ch1)

		b.Broadcast(event)

		assert.Len(t, ch1, 0)
		assert.Equal(t, event, <-ch2)
	})
}
