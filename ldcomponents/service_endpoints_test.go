package ldcomponents

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRelayProxyEndpoints(t *testing.T) {
	uri := "http://relay:8080"
	e := RelayProxyEndpoints(uri)
	assert.Equal(t, uri, e.Streaming)
	assert.Equal(t, uri, e.Polling)
	assert.Equal(t, uri, e.Events)
}

func TestRelayProxyEndpointsWithoutEvents(t *testing.T) {
	uri := "http://relay:8080"
	e := RelayProxyEndpointsWithoutEvents(uri)
	assert.Equal(t, uri, e.Streaming)
	assert.Equal(t, uri, e.Polling)
	assert.Equal(t, "", e.Events)
}
