package ldfiledatav2

import (
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncSkipsUnchangedFileAndDeliversChangedFile(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": false}}}`), func(path string) {
		var trigger func()
		f := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
			trigger = reload
			return nil
		}
		sync, err := DataSource().FilePaths(path).Reloader(f).Build(subsystems.BasicClientContext{})
		require.NoError(t, err)
		defer sync.Close()

		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))
		result := <-resultChan
		require.Equal(t, interfaces.DataSourceStateValid, result.State)

		// A change notification for an unchanged file must not deliver another basis.
		trigger()
		select {
		case extra := <-resultChan:
			require.Failf(t, "unexpected result", "an unchanged file was delivered again: %+v", extra)
		case <-time.After(500 * time.Millisecond): // several debounce windows
		}

		// A change notification for changed content must deliver the new basis.
		require.NoError(t, os.WriteFile(path, []byte(`{"flags": {"my-flag": {"on": true}}}`), 0600))
		trigger()
		select {
		case result = <-resultChan:
		case <-time.After(2 * time.Second):
			require.Fail(t, "the changed file was never delivered")
		}
		require.Equal(t, interfaces.DataSourceStateValid, result.State)
		require.NotNil(t, result.ChangeSet)
		require.Len(t, result.ChangeSet.Changes(), 1)
		assert.Equal(t, "my-flag", result.ChangeSet.Changes()[0].Key)
		assert.Contains(t, string(result.ChangeSet.Changes()[0].Object), `"on":true`)
	})
}

func TestCloseStopsTheReloadWorker(t *testing.T) {
	th.WithTempFileData([]byte(`{"flags": {"my-flag": {"on": false}}}`), func(path string) {
		baseline := runtime.NumGoroutine()

		var trigger func()
		f := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
			trigger = reload
			return nil
		}
		sync, err := DataSource().FilePaths(path).Reloader(f).Build(subsystems.BasicClientContext{})
		require.NoError(t, err)

		resultChan := sync.Sync(mocks.NewMockDataSelector(subsystems.NoSelector()))
		result := <-resultChan
		require.Equal(t, interfaces.DataSourceStateValid, result.State)
		_ = trigger

		require.NoError(t, sync.Close())
		th.AssertChannelClosed(t, resultChan, time.Second, "result channel should be closed")

		// Close must stop the reload worker goroutine, not only the result loop and the
		// change-signal source. Wait for the goroutine count to settle back to where it
		// started; a surviving worker keeps it above the baseline.
		deadline := time.Now().Add(2 * time.Second)
		for runtime.NumGoroutine() > baseline {
			if time.Now().After(deadline) {
				require.Failf(t, "goroutine leak", "goroutines: started at %d, still %d after Close",
					baseline, runtime.NumGoroutine())
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}
