package ldfiledata

import (
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	th "github.com/launchdarkly/go-test-helpers/v3"
	"github.com/stretchr/testify/require"
)

// countingSink counts Init calls so a test can tell a real reload from a skipped one.
type countingSink struct {
	initCalls atomic.Int32
}

func (s *countingSink) Init([]ldstoretypes.Collection) bool {
	s.initCalls.Add(1)
	return true
}
func (s *countingSink) Upsert(ldstoretypes.DataKind, string, ldstoretypes.ItemDescriptor) bool {
	return true
}
func (s *countingSink) UpdateStatus(interfaces.DataSourceState, interfaces.DataSourceErrorInfo) {}
func (s *countingSink) GetDataStoreStatusProvider() interfaces.DataStoreStatusProvider {
	return nil
}

func TestTriggeredReloadSkipsUnchangedFileAndAppliesChangedFile(t *testing.T) {
	th.WithTempFileData([]byte(`{"flagValues": {"flag1": true}}`), func(path string) {
		sink := &countingSink{}
		mockLog := ldlogtest.NewMockLog()
		testContext := sharedtest.NewTestContext("", nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
		testContext.DataSourceUpdateSink = sink

		var trigger func()
		factory := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
			trigger = reload
			return nil
		}
		ds, err := DataSource().FilePaths(path).Reloader(factory).Build(testContext)
		require.NoError(t, err)
		defer ds.Close()
		ready := make(chan struct{})
		ds.Start(ready)
		<-ready
		require.Equal(t, int32(1), sink.initCalls.Load())

		// A change notification for an unchanged file must not apply the data again.
		trigger()
		time.Sleep(500 * time.Millisecond) // several debounce windows
		require.Equal(t, int32(1), sink.initCalls.Load(),
			"an unchanged file was applied again on a triggered reload")

		// A change notification for changed content must apply it.
		require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"flag1": false}}`), 0600))
		trigger()
		deadline := time.Now().Add(2 * time.Second)
		for sink.initCalls.Load() != 2 {
			if time.Now().After(deadline) {
				require.Fail(t, "the changed file was never applied after a triggered reload")
			}
			time.Sleep(10 * time.Millisecond)
		}
	})
}

func TestCloseStopsTheReloadOrchestrator(t *testing.T) {
	th.WithTempFileData([]byte(`{"flagValues": {"flag1": true}}`), func(path string) {
		mockLog := ldlogtest.NewMockLog()
		testContext := sharedtest.NewTestContext("", nil, &subsystems.LoggingConfiguration{Loggers: mockLog.Loggers})
		store, _ := ldcomponents.InMemoryDataStore().Build(testContext)
		updates := mocks.NewMockDataSourceUpdates(store)
		testContext.DataSourceUpdateSink = updates

		var trigger func()
		factory := func(paths []string, loggers ldlog.Loggers, reload func(), closeCh <-chan struct{}) error {
			trigger = reload
			return nil
		}
		ds, err := DataSource().FilePaths(path).Reloader(factory).Build(testContext)
		require.NoError(t, err)
		ready := make(chan struct{})
		ds.Start(ready)
		<-ready

		require.NoError(t, ds.Close())

		// A change notification that arrives after Close must never reach the store: Close
		// stops the reload orchestrator, not only the change-signal source.
		require.NoError(t, os.WriteFile(path, []byte(`{"flagValues": {"flag1": false}}`), 0600))
		trigger()
		time.Sleep(500 * time.Millisecond) // several debounce windows

		flag := requireFlag(t, updates.DataStore, "flag1")
		require.Equal(t, true, flag.Variations[0].BoolValue(),
			"a reload ran after Close and re-initialized the store")
	})
}
