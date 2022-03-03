package internal

import (
	"testing"

	"gopkg.in/launchdarkly/go-server-sdk.v6/interfaces"
	"gopkg.in/launchdarkly/go-server-sdk.v6/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestClientContextImpl(t *testing.T) {
	sdkKey := "SDK_KEY"
	http := sharedtest.TestHTTPConfig()
	logging := sharedtest.TestLoggingConfig()

	basic1 := interfaces.BasicConfiguration{SDKKey: sdkKey}
	context1 := NewClientContextImpl(basic1, http, logging)
	assert.Equal(t, sdkKey, context1.GetBasic().SDKKey)
	assert.False(t, context1.GetBasic().Offline)
	assert.Equal(t, http, context1.GetHTTP())
	assert.Equal(t, logging, context1.GetLogging())
	assert.Nil(t, context1.DiagnosticsManager)
}
