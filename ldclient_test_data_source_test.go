package ldclient

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-server-sdk.v5/ldcomponents"
	"gopkg.in/launchdarkly/go-server-sdk.v5/testhelpers/ldtestdata"
)

// This is a basic smoke test to verify that TestDataSource works correctly inside of an LDClient instance.
// It is in the main package, rather than the testhelpers package, to avoid a circular package reference.

func TestClientWithTestDataSource(t *testing.T) {
	td := ldtestdata.DataSource()
	td.Update(td.Flag("flagkey").On(true))

	config := Config{
		DataSource: td,
		Events:     ldcomponents.NoEvents(),
	}
	client, err := MakeCustomClient("", config, time.Second)
	require.NoError(t, err)
	defer client.Close()

	value, err := client.BoolVariation("flagkey", lduser.NewUser("userkey"), false)
	require.NoError(t, err)
	assert.True(t, value)

	td.Update(td.Flag("flagkey").On(false))
	value, err = client.BoolVariation("flagkey", lduser.NewUser("userkey"), false)
	require.NoError(t, err)
	assert.False(t, value)
}
