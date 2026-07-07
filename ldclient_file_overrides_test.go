package ldclient

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldoverrides"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test drives the file-based override source through a running client: an operator
// writes, edits, and empties an override file, and evaluations follow without any client
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

	// An operator adds an override; the running client picks it up.
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"overridden-flag": true}}`), 0600))
	require.Eventually(t, evaluatesTo(true), 10*time.Second, 50*time.Millisecond)

	// The override changes value.
	require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"overridden-flag": false}}`), 0600))
	require.Eventually(t, func() bool {
		_, detail, _ := client.BoolVariationDetail("overridden-flag", evalTestUser, true)
		return detail.Reason.IsOverride() && detail.Value.BoolValue() == false
	}, 10*time.Second, 50*time.Millisecond)

	// The override is removed; the not-initialized short-circuit returns.
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	require.Eventually(t, func() bool {
		_, err := client.BoolVariation("overridden-flag", evalTestUser, false)
		return err == ErrClientNotInitialized
	}, 10*time.Second, 50*time.Millisecond)
}
