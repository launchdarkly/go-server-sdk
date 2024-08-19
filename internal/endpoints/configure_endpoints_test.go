package endpoints

import (
	"fmt"
	"strings"
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestDefaultURISelectedIfNoCustomURISpecified(t *testing.T) {
	logger := ldlogtest.NewMockLog()
	endpoints := interfaces.ServiceEndpoints{}
	services := []ServiceType{StreamingService, PollingService, EventsService}
	for _, service := range services {
		assert.Equal(t, strings.TrimSuffix(DefaultBaseURI(service), "/"), SelectBaseURI(endpoints, service, logger.Loggers))
	}
}

func TestSelectCustomURIs(t *testing.T) {
	logger := ldlogtest.NewMockLog()
	const customURI = "http://custom_uri"

	cases := []struct {
		endpoints interfaces.ServiceEndpoints
		service   ServiceType
	}{
		{interfaces.ServiceEndpoints{Polling: customURI}, PollingService},
		{interfaces.ServiceEndpoints{Streaming: customURI}, StreamingService},
		{interfaces.ServiceEndpoints{Events: customURI}, EventsService},
	}

	for _, c := range cases {
		assert.Equal(t, customURI, SelectBaseURI(c.endpoints, c.service, logger.Loggers))
	}

	assert.Empty(t, logger.GetOutput(ldlog.Error))
}

func TestLogErrorIfAtLeastOneButNotAllCustomURISpecified(t *testing.T) {
	const customURI = "http://custom_uri"

	cases := []struct {
		endpoints interfaces.ServiceEndpoints
		service   ServiceType
	}{
		{interfaces.ServiceEndpoints{Streaming: customURI}, PollingService},
		{interfaces.ServiceEndpoints{Events: customURI}, PollingService},
		{interfaces.ServiceEndpoints{Streaming: customURI, Events: customURI}, PollingService},

		{interfaces.ServiceEndpoints{Polling: customURI}, StreamingService},
		{interfaces.ServiceEndpoints{Events: customURI}, StreamingService},
		{interfaces.ServiceEndpoints{Polling: customURI, Events: customURI}, StreamingService},

		{interfaces.ServiceEndpoints{Streaming: customURI}, EventsService},
		{interfaces.ServiceEndpoints{Polling: customURI}, EventsService},
		{interfaces.ServiceEndpoints{Streaming: customURI, Polling: customURI}, EventsService},
	}

	t.Run("without explicit partial specification", func(t *testing.T) {
		logger := ldlogtest.NewMockLog()

		// Even if the configuration is considered to be likely malformed, we should still return the proper default URI for
		// the service that wasn't configured.
		for _, c := range cases {
			assert.Equal(t, strings.TrimSuffix(DefaultBaseURI(c.service), "/"), SelectBaseURI(c.endpoints, c.service, logger.Loggers))
		}

		// For each service that wasn't configured, we should see a log message indicating that.
		for _, c := range cases {
			logger.AssertMessageMatch(t, true, ldlog.Error, fmt.Sprintf("You have set custom ServiceEndpoints without specifying the %s base URI", c.service))
		}
	})

	t.Run("with partial specification", func(t *testing.T) {
		logger := ldlogtest.NewMockLog()

		for _, c := range cases {
			endpoints := c.endpoints.WithPartialSpecification()
			assert.Equal(t, strings.TrimSuffix(DefaultBaseURI(c.service), "/"), SelectBaseURI(endpoints, c.service, logger.Loggers))
		}
		assert.Empty(t, logger.GetOutput(ldlog.Error))
	})
}
