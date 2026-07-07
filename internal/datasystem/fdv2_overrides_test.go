package datasystem

import (
	"testing"
	"time"

	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces"
	"github.com/launchdarkly/go-server-sdk/v7/internal"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/overrides"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixedConfig subsystems.DataSystemConfiguration

func (f fixedConfig) Build(subsystems.ClientContext) (subsystems.DataSystemConfiguration, error) {
	return subsystems.DataSystemConfiguration(f), nil
}

func makeOverrideTestFDv2(t *testing.T, disabled bool, source subsystems.OverrideSource) *FDv2 {
	t.Helper()
	clientContext := &internal.ClientContextImpl{
		BasicClientContext: subsystems.BasicClientContext{
			SDKKey:  sharedtest.TestSDKKey,
			Logging: sharedtest.TestLoggingConfig(),
		},
	}
	system, err := NewFDv2(disabled, fixedConfig{OverrideSource: source}, clientContext, nil)
	require.NoError(t, err)
	return system
}

func startAndWait(t *testing.T, system *FDv2) {
	t.Helper()
	ready := make(chan struct{})
	system.Start(ready)
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for data system to start")
	}
	t.Cleanup(func() { _ = system.Stop() })
}

func overrideFlagData(flags ...ldmodel.FeatureFlag) []st.Collection {
	coll := st.Collection{Kind: datakinds.Features}
	for _, flag := range flags {
		coll.Items = append(coll.Items,
			st.KeyedItemDescriptor{Key: flag.Key, Item: sharedtest.FlagDescriptor(flag)})
	}
	return []st.Collection{coll}
}

func TestFDv2WithoutOverrideSourceServesRawStore(t *testing.T) {
	system := makeOverrideTestFDv2(t, false, nil)
	_, isRawStore := system.Store().(*Store)
	assert.True(t, isRawStore, "Store() should be the raw store when no override source is configured")
	assert.False(t, system.HasOverrides())
	assert.False(t, system.HasFlagOverride("anything"))
}

func TestFDv2WithOverrideSourceServesOverlay(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideFlagData(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()))
	system := makeOverrideTestFDv2(t, false, source)

	_, isOverlay := system.Store().(*overrides.Overlay)
	assert.True(t, isOverlay, "Store() should be the overlay when an override source is configured")

	// The source is not started (and the layer is empty) until Start.
	assert.False(t, system.HasOverrides())

	startAndWait(t, system)

	assert.True(t, source.IsStarted())
	assert.True(t, system.HasOverrides())
	assert.True(t, system.HasFlagOverride("flag1"))
	assert.False(t, system.HasFlagOverride("flag2"))

	item, err := system.Store().Get(datakinds.Features, "flag1")
	require.NoError(t, err)
	flag, ok := item.Item.(*ldmodel.FeatureFlag)
	require.True(t, ok)
	assert.True(t, flag.IsOverride)
}

func TestFDv2OverridesDoNotAffectDataAvailability(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideFlagData(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()))
	system := makeOverrideTestFDv2(t, false, source)

	availabilityBefore := system.DataAvailability()
	startAndWait(t, system)
	assert.Equal(t, availabilityBefore, system.DataAvailability())
}

func TestFDv2OverrideUpdatesTriggerFlagChangeEvents(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(nil)
	system := makeOverrideTestFDv2(t, false, source)
	listener := system.FlagChangeEventBroadcaster().AddListener()
	startAndWait(t, system)

	source.SetOverrides(overrideFlagData(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()))

	select {
	case event := <-listener:
		assert.Equal(t, interfaces.FlagChangeEvent{Key: "flag1"}, event)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for flag change event")
	}
}

func TestFDv2StopClosesOverrideSource(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(nil)
	system := makeOverrideTestFDv2(t, false, source)
	ready := make(chan struct{})
	system.Start(ready)
	<-ready

	require.NoError(t, system.Stop())
	assert.True(t, source.IsClosed())
}

func TestFDv2DisabledDoesNotStartOverrideSource(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideFlagData(ldbuilders.NewFlagBuilder("flag1").Version(1).Build()))
	system := makeOverrideTestFDv2(t, true, source)
	startAndWait(t, system)

	assert.False(t, source.IsStarted())
	assert.False(t, system.HasOverrides())
	_, isRawStore := system.Store().(*Store)
	assert.True(t, isRawStore)
}

func TestFDv1HasNoOverrides(t *testing.T) {
	system := &FDv1{}
	assert.False(t, system.HasFlagOverride("anything"))
	assert.False(t, system.HasOverrides())
}
