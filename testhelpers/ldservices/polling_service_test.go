package ldservices

import (
	"bytes"
	"io"
	"testing"

	"github.com/launchdarkly/go-test-helpers/v2/httphelpers"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServerSidePollingEndpoint(t *testing.T) {
	data := "fake data"
	handler := ServerSidePollingServiceHandler(data)
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Get(serverSideSDKPollingPath)
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	bytes, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, `"fake data"`, string(bytes)) // the extra quotes are because the value was marshalled to JSON
}

func TestServerSidePollingReturns404ForWrongURL(t *testing.T) {
	data := "fake data"
	handler := ServerSidePollingServiceHandler(data)
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Get("/other/path")
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 404, resp.StatusCode)
}

func TestServerSidePollingReturns405ForWrongMethod(t *testing.T) {
	data := "fake data"
	handler := ServerSidePollingServiceHandler(data)
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Post(serverSideSDKPollingPath, "text/plain", bytes.NewBufferString("hello"))
	assert.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, 405, resp.StatusCode)
}

func TestServerSidePollingMarshalsDataAgainForEachRequest(t *testing.T) {
	data := NewServerSDKData()
	handler := ServerSidePollingServiceHandler(data)
	client := httphelpers.ClientFromHandler(handler)

	resp, err := client.Get(serverSideSDKPollingPath)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	bytes, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, `{"flags":{},"segments":{}}`, string(bytes))

	data.Flags(FlagOrSegment("flagkey", 1))
	resp, err = client.Get(serverSideSDKPollingPath)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	bytes, err = io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, `{"flags":{"flagkey":{"key":"flagkey","version":1}},"segments":{}}`, string(bytes))
}
