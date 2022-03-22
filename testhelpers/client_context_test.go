package testhelpers

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"

	"github.com/stretchr/testify/assert"
)

func TestSimpleClientContext(t *testing.T) {
	c := NewSimpleClientContext("key")
	assert.Equal(t, "key", c.GetBasic().SDKKey)
	assert.False(t, c.GetBasic().Offline)

	// Note, can't test equality of HTTPConfiguration because it contains a function
	hc, _ := ldcomponents.HTTPConfiguration().CreateHTTPConfiguration(c.GetBasic())
	assert.Equal(t, hc.DefaultHeaders, c.GetHTTP().DefaultHeaders)

	lc, _ := ldcomponents.Logging().CreateLoggingConfiguration(c.GetBasic())
	assert.Equal(t, lc, c.GetLogging())

	h := ldcomponents.HTTPConfiguration().UserAgent("u").Wrapper("w", "")
	hc1, _ := h.CreateHTTPConfiguration(c.GetBasic())
	assert.Equal(t, hc1.DefaultHeaders, c.WithHTTP(h).GetHTTP().DefaultHeaders)

	l := ldcomponents.Logging().Loggers(ldlog.NewDefaultLoggers()).MinLevel(ldlog.Debug)
	lc1, _ := l.CreateLoggingConfiguration(c.GetBasic())
	assert.Equal(t, lc1, c.WithLogging(l).GetLogging())
}
