package ldservices

import (
	"bytes"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSideEventsEndpoint(t *testing.T) {
	handler := ServerSideEventsServiceHandler()
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Post("http://fake/bulk", "text/plain", bytes.NewBufferString("hello"))
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 202, resp.StatusCode)
}

func TestServerSideDiagnosticEventsEndpoint(t *testing.T) {
	handler := ServerSideEventsServiceHandler()
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Post("http://fake/diagnostic", "text/plain", bytes.NewBufferString("hello"))
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 202, resp.StatusCode)
}

func TestServerSideEventsHandlerReturns404ForWrongURL(t *testing.T) {
	handler := ServerSideEventsServiceHandler()
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Post("http://fake/other", "text/plain", bytes.NewBufferString("hello"))
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestServerSideEventsEndpointReturns405ForWrongMethod(t *testing.T) {
	handler := ServerSideEventsServiceHandler()
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Get("http://fake/bulk")
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 405, resp.StatusCode)
}

func TestServerSideDiagnosticEventsEndpointReturns405ForWrongMethod(t *testing.T) {
	handler := ServerSideEventsServiceHandler()
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Get("http://fake/diagnostic")
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 405, resp.StatusCode)
}
