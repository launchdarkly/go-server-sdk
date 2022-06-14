package internal

import (
	"net/http"
	"testing"

	"github.com/launchdarkly/go-server-sdk/v6/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v6/subsystems"

	"github.com/stretchr/testify/assert"
)

func TestClientContextImpl(t *testing.T) {
	sdkKey := "SDK_KEY"
	http := subsystems.HTTPConfiguration{DefaultHeaders: make(http.Header)}
	logging := sharedtest.TestLoggingConfig()

	basic1 := subsystems.BasicConfiguration{SDKKey: sdkKey}
	context1 := NewClientContextImpl(basic1, http, logging)
	assert.Equal(t, sdkKey, context1.GetBasic().SDKKey)
	assert.False(t, context1.GetBasic().Offline)
	assert.Equal(t, http.DefaultHeaders, context1.GetHTTP().DefaultHeaders)
	assert.NotNil(t, context1.GetHTTP().CreateHTTPClient)
	assert.Equal(t, logging, context1.GetLogging())
	assert.Nil(t, context1.DiagnosticsManager)
}
