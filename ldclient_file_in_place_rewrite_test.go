package ldclient

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/ldfiledata"
	"github.com/launchdarkly/go-server-sdk/v7/ldfiledatav2"
	"github.com/launchdarkly/go-server-sdk/v7/ldfilewatch"
	"github.com/launchdarkly/go-server-sdk/v7/ldoverrides"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The tests in this file rewrite a data file in place while a client serves its flags. The file
// is empty for 300 ms, longer than the settle window after a change, so a reload reads it empty.
// The client must keep serving the old value until the new one is written.

// inPlaceRewriteContents is the content of a file that serves "a" for flag1, and of the same
// file rewritten to serve "b", in each format.
var inPlaceRewriteContents = map[string][2]string{
	".json": {`{"flagValues": {"flag1": "a"}}`, `{"flagValues": {"flag1": "b"}}`},
	".yaml": {"flagValues:\n  flag1: a\n", "flagValues:\n  flag1: b\n"},
}

// servedDuringInPlaceRewrite rewrites the file in place and returns the values that the client
// served for flag1 meanwhile, in order and without repeats, up to the rewritten value.
func servedDuringInPlaceRewrite(t *testing.T, client *LDClient, path string, content string) []string {
	t.Helper()
	var mu sync.Mutex
	var served []string
	lastServed := func() string {
		mu.Lock()
		defer mu.Unlock()
		if len(served) == 0 {
			return ""
		}
		return served[len(served)-1]
	}
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		for {
			select {
			case <-stop:
				return
			default:
			}
			value, _ := client.StringVariation("flag1", evalTestUser, "default")
			if value != lastServed() {
				mu.Lock()
				served = append(served, value)
				mu.Unlock()
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	defer func() {
		close(stop)
		<-sampled
	}()

	sharedtest.RewriteFileInPlace(t, path, content, 300*time.Millisecond)

	// Waits for the sampler itself to see the new value, so it has recorded everything that the
	// client served before it.
	require.Eventually(t, func() bool { return lastServed() == "b" }, 10*time.Second, 10*time.Millisecond,
		"the client never served the rewritten file")
	mu.Lock()
	defer mu.Unlock()
	return append([]string(nil), served...)
}

func writeInPlaceRewriteFile(t *testing.T, name string, extension string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name+extension)
	require.NoError(t, os.WriteFile(path, []byte(inPlaceRewriteContents[extension][0]), 0600))
	return path
}

func TestFileDataSourceInPlaceRewriteNeverServesAnEmptyFile(t *testing.T) {
	for _, extension := range []string{".json", ".yaml"} {
		t.Run(extension, func(t *testing.T) {
			path := writeInPlaceRewriteFile(t, "flags", extension)
			config := Config{
				Logging:    ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
				Events:     ldcomponents.NoEvents(),
				DataSource: ldfiledata.DataSource().FilePaths(path).Reloader(ldfilewatch.WatchFiles),
			}
			client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
			require.NoError(t, err)
			defer client.Close()
			value, _ := client.StringVariation("flag1", evalTestUser, "default")
			require.Equal(t, "a", value)

			served := servedDuringInPlaceRewrite(t, client, path, inPlaceRewriteContents[extension][1])
			assert.Equal(t, []string{"a", "b"}, served)
		})
	}
}

func TestFileDataSynchronizerInPlaceRewriteNeverServesAnEmptyFile(t *testing.T) {
	for _, extension := range []string{".json", ".yaml"} {
		t.Run(extension, func(t *testing.T) {
			path := writeInPlaceRewriteFile(t, "flags", extension)
			config := Config{
				Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
				Events:  ldcomponents.NoEvents(),
				DataSystem: ldcomponents.DataSystem().Custom().
					Synchronizers(ldfiledatav2.DataSource().FilePaths(path).Reloader(ldfilewatch.WatchFiles)),
			}
			client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
			require.NoError(t, err)
			defer client.Close()
			value, _ := client.StringVariation("flag1", evalTestUser, "default")
			require.Equal(t, "a", value)

			served := servedDuringInPlaceRewrite(t, client, path, inPlaceRewriteContents[extension][1])
			assert.Equal(t, []string{"a", "b"}, served)
		})
	}
}

func TestFileOverridesInPlaceRewriteNeverServesAnEmptyFile(t *testing.T) {
	for _, extension := range []string{".json", ".yaml"} {
		t.Run(extension, func(t *testing.T) {
			path := writeInPlaceRewriteFile(t, "overrides", extension)
			config := Config{
				Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
				Events:  ldcomponents.NoEvents(),
				DataSystem: ldcomponents.DataSystem().Custom().
					Synchronizers(newHangingSynchronizer()).
					Overrides(ldoverrides.FileSource().FilePaths(path).ChangeDetection(ldoverrides.Watching)),
			}
			client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
			require.NotNil(t, client)
			defer client.Close()
			value, _ := client.StringVariation("flag1", evalTestUser, "default")
			require.Equal(t, "a", value)

			// Without the override the client, which has no LaunchDarkly data, serves the default.
			served := servedDuringInPlaceRewrite(t, client, path, inPlaceRewriteContents[extension][1])
			assert.Equal(t, []string{"a", "b"}, served)
		})
	}
}
