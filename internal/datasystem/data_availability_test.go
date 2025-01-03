package datasystem

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDataAvailibilityAtLeast(t *testing.T) {
	assert.True(t, Refreshed.AtLeast(Refreshed))
	assert.True(t, Refreshed.AtLeast(Cached))
	assert.True(t, Refreshed.AtLeast(Defaults))

	assert.False(t, Cached.AtLeast(Refreshed))
	assert.True(t, Cached.AtLeast(Cached))
	assert.True(t, Cached.AtLeast(Defaults))

	assert.False(t, Defaults.AtLeast(Refreshed))
	assert.False(t, Defaults.AtLeast(Cached))
	assert.True(t, Defaults.AtLeast(Defaults))
}
