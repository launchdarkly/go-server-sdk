package internal

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHttpConfigurationImpl(t *testing.T) {
	t.Run("GetDefaultHeaders", func(t *testing.T) {
		h0 := make(http.Header)
		h0.Set("a", "1")

		hc := HTTPConfigurationImpl{DefaultHeaders: h0}

		h1 := hc.GetDefaultHeaders()
		assert.Equal(t, "1", h1.Get("a"))

		h1.Set("a", "2") // verify that this is a copy, not the original
		h2 := hc.GetDefaultHeaders()
		assert.Equal(t, "1", h2.Get("a"))
	})

	t.Run("CreateHTTPClient", func(t *testing.T) {
		hc1 := HTTPConfigurationImpl{}
		client1 := hc1.CreateHTTPClient()
		assert.NotNil(t, client1)

		client2 := *http.DefaultClient
		client2.Timeout = time.Hour
		hc2 := HTTPConfigurationImpl{HTTPClientFactory: func() *http.Client { return &client2 }}
		client3 := hc2.CreateHTTPClient()
		assert.Equal(t, client2, *client3)
	})
}
