package ldcomponents

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPollingDataSourceBuilder(t *testing.T) {
	t.Run("BaseURI", func(t *testing.T) {
		p := PollingDataSource()
		assert.Equal(t, DefaultPollingBaseURI, p.baseURI)

		p.BaseURI("x")
		assert.Equal(t, "x", p.baseURI)

		p.BaseURI("")
		assert.Equal(t, DefaultPollingBaseURI, p.baseURI)
	})

	t.Run("PollInterval", func(t *testing.T) {
		p := PollingDataSource()
		assert.Equal(t, DefaultPollInterval, p.pollInterval)

		p.PollInterval(time.Hour)
		assert.Equal(t, time.Hour, p.pollInterval)

		p.PollInterval(time.Second)
		assert.Equal(t, DefaultPollInterval, p.pollInterval)

		p.forcePollInterval(time.Second)
		assert.Equal(t, time.Second, p.pollInterval)
	})
}
