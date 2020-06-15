package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestStreamingDataSourceBuilder(t *testing.T) {
	t.Run("BaseURI", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, DefaultStreamingBaseURI, s.baseURI)

		s.BaseURI("x")
		assert.Equal(t, "x", s.baseURI)

		s.BaseURI("")
		assert.Equal(t, DefaultStreamingBaseURI, s.baseURI)
	})

	t.Run("PollingBaseURI", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, "", s.pollingBaseURI)

		s.PollingBaseURI("x")
		assert.Equal(t, "x", s.pollingBaseURI)

		s.PollingBaseURI("")
		assert.Equal(t, "", s.pollingBaseURI)
	})

	t.Run("InitialReconnectDelay", func(t *testing.T) {
		s := StreamingDataSource()
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(time.Minute)
		assert.Equal(t, time.Minute, s.initialReconnectDelay)

		s.InitialReconnectDelay(0)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)

		s.InitialReconnectDelay(-1 * time.Millisecond)
		assert.Equal(t, DefaultInitialReconnectDelay, s.initialReconnectDelay)
	})
}
