package datastore

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal"
)

func TestDataStoreUpdatesImpl(t *testing.T) {
	t.Run("getStatus", func(t *testing.T) {
		dataStoreUpdates := NewDataStoreUpdatesImpl(internal.NewDataStoreStatusBroadcaster())

		assert.Equal(t, interfaces.DataStoreStatus{Available: true}, dataStoreUpdates.getStatus())

		newStatus := interfaces.DataStoreStatus{Available: true}
		dataStoreUpdates.UpdateStatus(newStatus)

		assert.Equal(t, newStatus, dataStoreUpdates.getStatus())
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		broadcaster := internal.NewDataStoreStatusBroadcaster()
		defer broadcaster.Close()

		ch := broadcaster.AddListener()

		dataStoreUpdates := NewDataStoreUpdatesImpl(broadcaster)

		newStatus := interfaces.DataStoreStatus{Available: false}
		dataStoreUpdates.UpdateStatus(newStatus)

		assert.Equal(t, newStatus, <-ch)
	})
}
