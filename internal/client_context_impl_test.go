package internal

import (
	"net/http"
	"testing"
	"time"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	ldevents "gopkg.in/launchdarkly/go-sdk-events.v1"
	"gopkg.in/launchdarkly/go-server-sdk.v5/sharedtest"

	"github.com/stretchr/testify/assert"
)

func TestClientContextImpl(t *testing.T) {
	sdkKey := "SDK_KEY"
	logging := sharedtest.TestLogging()
	headers := make(http.Header)
	headers.Set("x", "y")

	context1 := NewClientContextImpl(sdkKey, logging, headers, nil, false, nil)
	assert.Equal(t, sdkKey, context1.GetSDKKey())
	assert.Equal(t, logging, context1.GetLogging())
	assert.Equal(t, headers, context1.GetDefaultHTTPHeaders())
	assert.NotNil(t, context1.CreateHTTPClient())
	assert.False(t, context1.IsOffline())
	assert.Nil(t, context1.(*clientContextImpl).GetDiagnosticsManager())

	httpClient := &http.Client{}
	diagnosticsManager := ldevents.NewDiagnosticsManager(ldvalue.Null(), ldvalue.Null(), ldvalue.Null(), time.Now(), nil)
	context2 := NewClientContextImpl(sdkKey, logging, headers, func() *http.Client { return httpClient }, true, diagnosticsManager)
	assert.Equal(t, httpClient, context2.CreateHTTPClient())
	assert.True(t, context2.IsOffline())
	assert.Equal(t, diagnosticsManager, context2.(*clientContextImpl).GetDiagnosticsManager())
}
