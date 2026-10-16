package datastore

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	th "github.com/launchdarkly/go-test-helpers/v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCacheExporter stands in for the in-memory store that holds the authoritative copy of the data.
type fakeCacheExporter struct {
	data  []st.Collection
	calls int
	lock  sync.Mutex
}

func (f *fakeCacheExporter) ExportAll() ([]st.Collection, bool) {
	f.lock.Lock()
	defer f.lock.Unlock()
	f.calls++
	return f.data, f.data != nil
}

func (f *fakeCacheExporter) exportCalls() int {
	f.lock.Lock()
	defer f.lock.Unlock()
	return f.calls
}

// When an outage ends, the wrapper writes the registered exporter's data to the store before it
// reports the store available, and it asks for no refresh.
func TestPersistentDataStoreWrapperRecoveryFromCacheExporter(t *testing.T) {
	const statusTimeout = 2 * time.Second // the status poller runs every 500ms

	fakeError := errors.New("sorry")
	flagV1 := ldbuilders.NewFlagBuilder("flag").Version(1).Build()
	flagV2 := ldbuilders.NewFlagBuilder("flag").Version(2).Build()
	segment := ldbuilders.NewSegmentBuilder("segment").Version(7).Build()

	for _, mode := range []testCacheMode{testUncached, testCached, testCachedIndefinitely} {
		t.Run(string(mode), func(t *testing.T) {
			withDataStoreStatusTestParams(mode, func(p dataStoreStatusTestParams) {
				w := p.store.(*persistentDataStoreWrapper)
				statusCh := p.broadcaster.AddListener()
				require.NoError(t, p.store.Init(sharedtest.NewDataSetBuilder().Flags(flagV1).Build()))

				exporter := &fakeCacheExporter{
					data: sharedtest.NewDataSetBuilder().Flags(flagV2).Segments(segment).Build(),
				}

				// The data system registers its in-memory store, then drops the wrapper's cache,
				// once that store holds a basis.
				w.SetCacheExporter(exporter)
				w.DropCache()

				// Fail a write so that the wrapper sees the store as unavailable.
				p.core.SetFakeError(fakeError)
				p.core.SetAvailable(false)
				_, err := p.store.Upsert(datakinds.Features, flagV2.Key, sharedtest.FlagDescriptor(flagV2))
				require.Equal(t, fakeError, err)
				require.Equal(t, interfaces.DataStoreStatus{Available: false},
					th.RequireValue(t, statusCh, statusTimeout))

				// Bring the store back up.
				p.core.SetFakeError(nil)
				p.core.SetAvailable(true)

				// The wrapper writes every kind of exported data before it reports recovery, and
				// asks for no refresh because it has already done one.
				assert.Equal(t, interfaces.DataStoreStatus{Available: true},
					th.RequireValue(t, statusCh, statusTimeout))
				assert.Equal(t, flagV2.Version, p.core.ForceGet(datakinds.Features, flagV2.Key).Version)
				assert.Equal(t, segment.Version, p.core.ForceGet(datakinds.Segments, segment.Key).Version)
			})
		})
	}
}
