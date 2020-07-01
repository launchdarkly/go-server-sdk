package internal

import (
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/internal/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestClientContextImpl(t *testing.T) {
	sdkKey := "SDK_KEY"
	http := sharedtest.TestHTTPConfig()
	logging := sharedtest.TestLoggingConfig()

	context1 := NewClientContextImpl(sdkKey, http, logging, false, nil)
	assert.Equal(t, sdkKey, context1.GetBasic().SDKKey)
	assert.False(t, context1.GetBasic().Offline)
	assert.Equal(t, http, context1.GetHTTP())
	assert.Equal(t, logging, context1.GetLogging())
	assert.Nil(t, context1.(*clientContextImpl).GetDiagnosticsManager())

	diagnosticsManager := ldevents.NewDiagnosticsManager(ldvalue.Null(), ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
	context2 := NewClientContextImpl(sdkKey, http, logging, true, diagnosticsManager)
	assert.Equal(t, sdkKey, context2.GetBasic().SDKKey)
	assert.True(t, context2.GetBasic().Offline)
	assert.Equal(t, diagnosticsManager, context2.(*clientContextImpl).GetDiagnosticsManager())
}
