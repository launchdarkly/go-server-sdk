package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/internal"
)

func TestDataStoreUpdateSinkImpl(t *testing.T) {
	t.Run("getStatus", func(t *testing.T) {
		dataStoreUpdates := NewDataStoreUpdateSinkImpl(internal.NewBroadcaster[interfaces.DataStoreStatus]())

		assert.Equal(t, interfaces.DataStoreStatus{Available: true}, dataStoreUpdates.getStatus())

		newStatus := interfaces.DataStoreStatus{Available: true}
		dataStoreUpdates.UpdateStatus(newStatus)

		assert.Equal(t, newStatus, dataStoreUpdates.getStatus())
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		broadcaster := internal.NewBroadcaster[interfaces.DataStoreStatus]()
		defer broadcaster.Close()

		ch := broadcaster.AddListener()

		dataStoreUpdates := NewDataStoreUpdateSinkImpl(broadcaster)

		newStatus := interfaces.DataStoreStatus{Available: false}
		dataStoreUpdates.UpdateStatus(newStatus)

		assert.Equal(t, newStatus, <-ch)
	})
}
