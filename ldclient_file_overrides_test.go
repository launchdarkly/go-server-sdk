package ldclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldoverrides"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A "flagValues" entry is not available to client-side SDKs. A full definition that sets
// "clientSide" is, so that is how an operator overrides a flag that client-side SDKs use.
func TestFileOverridesClientSideAllFlagsStateKeepsOnlyClientSideDefinitions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"value-flag": true},
		"flags": {"full-flag": {"key": "full-flag", "version": 1, "on": true, "variations": [true],
		"fallthrough": {"variation": 0}, "clientSide": true}}}`), 0600))

	config := Config{
		Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
		Events:  ldcomponents.NoEvents(),
		DataSystem: ldcomponents.DataSystem().Custom().
			Synchronizers(newHangingSynchronizer()).
			Overrides(ldoverrides.FileSource().FilePaths(path)),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	require.NotNil(t, client)
	defer client.Close()

	all := client.AllFlagsState(evalTestUser).ToValuesMap()
	assert.Len(t, all, 2)
	assert.Contains(t, all, "value-flag")
	assert.Contains(t, all, "full-flag")

	clientSide := client.AllFlagsState(evalTestUser, flagstate.OptionClientSideOnly())
	assert.True(t, clientSide.IsValid())
	clientSideValues := clientSide.ToValuesMap()
	assert.Len(t, clientSideValues, 1)
	assert.Contains(t, clientSideValues, "full-flag")
}

// This test drives the file-based override source through a running client. An operator
// writes, edits, and empties an override file. Evaluations follow without any client
// restart, even though the client never obtains data from LaunchDarkly.
func TestFileOverridesEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "overrides.json")
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))

	config := Config{
		Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
		Events:  ldcomponents.NoEvents(),
		DataSystem: ldcomponents.DataSystem().Custom().
			Synchronizers(newHangingSynchronizer()).
			Overrides(ldoverrides.FileSource().FilePaths(path)),
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	require.NotNil(t, client)
	defer client.Close()

	evaluatesTo := func(expected bool) func() bool {
		return func() bool {
			value, _ := client.BoolVariation("overridden-flag", evalTestUser, false)
			return value == expected
		}
	}

	// Not initialized and no override present: the default is served.
	value, err := client.BoolVariation("overridden-flag", evalTestUser, false)
	assert.Equal(t, ErrClientNotInitialized, err)
	assert.False(t, value)

	// An operator adds an override. The running client picks it up.
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"overridden-flag": true}}`), 0600))
	require.Eventually(t, evaluatesTo(true), 10*time.Second, 50*time.Millisecond)

	// The override changes value.
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"overridden-flag": false}}`), 0600))
	require.Eventually(t, func() bool {
		_, detail, _ := client.BoolVariationDetail("overridden-flag", evalTestUser, true)
		return detail.Reason.IsOverrideAffected() && !detail.Value.BoolValue()
	}, 10*time.Second, 50*time.Millisecond)

	// The override is removed. The not-initialized short-circuit returns.
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	require.Eventually(t, func() bool {
		_, err := client.BoolVariation("overridden-flag", evalTestUser, false)
		return err == ErrClientNotInitialized
	}, 10*time.Second, 50*time.Millisecond)
}
