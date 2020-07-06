package testhelpers

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldlog"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
)

func TestSimpleClientContext(t *testing.T) {
	c := NewSimpleClientContext("key")
	assert.Equal(t, "key", c.GetBasic().SDKKey)
	assert.False(t, c.GetBasic().Offline)

	// Note, can't test equality of HTTPConfiguration because it contains a function
	hc, _ := ldcomponents.HTTPConfiguration().CreateHTTPConfiguration(c.GetBasic())
	assert.Equal(t, hc.GetDefaultHeaders(), c.GetHTTP().GetDefaultHeaders())

	lc, _ := ldcomponents.Logging().CreateLoggingConfiguration(c.GetBasic())
	assert.Equal(t, lc, c.GetLogging())

	h := ldcomponents.HTTPConfiguration().UserAgent("u").Wrapper("w", "")
	hc1, _ := h.CreateHTTPConfiguration(c.GetBasic())
	assert.Equal(t, hc1.GetDefaultHeaders(), c.WithHTTP(h).GetHTTP().GetDefaultHeaders())

	l := ldcomponents.Logging().Loggers(ldlog.NewDefaultLoggers()).MinLevel(ldlog.Debug)
	lc1, _ := l.CreateLoggingConfiguration(c.GetBasic())
	assert.Equal(t, lc1, c.WithLogging(l).GetLogging())
}
