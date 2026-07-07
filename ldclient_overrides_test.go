package ldclient

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldreason"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hangingSynchronizer is a DataSynchronizer that connects but never yields data, leaving
// the client permanently uninitialized.
type hangingSynchronizer struct {
	quit chan struct{}
}

func newHangingSynchronizer() *hangingSynchronizer {
	return &hangingSynchronizer{quit: make(chan struct{})}
}

func (h *hangingSynchronizer) Name() string { return "HangingSynchronizer" }

func (h *hangingSynchronizer) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error) {
	return nil, false, errors.New("no data available")
}

func (h *hangingSynchronizer) Sync(ds subsystems.DataSelector) <-chan subsystems.DataSynchronizerResult {
	results := make(chan subsystems.DataSynchronizerResult)
	go func() {
		<-h.quit
		close(results)
	}()
	return results
}

func (h *hangingSynchronizer) Close() error {
	close(h.quit)
	return nil
}

func (h *hangingSynchronizer) Build(subsystems.ClientContext) (subsystems.DataSynchronizer, error) {
	return h, nil
}

func overrideTestFlagData(flags ...ldmodel.FeatureFlag) []st.Collection {
	coll := st.Collection{Kind: datakinds.Features}
	for _, flag := range flags {
		coll.Items = append(coll.Items,
			st.KeyedItemDescriptor{Key: flag.Key, Item: sharedtest.FlagDescriptor(flag)})
	}
	return []st.Collection{coll}
}

func singleValueFlag(key string, value ldvalue.Value) ldmodel.FeatureFlag {
	return ldbuilders.NewFlagBuilder(key).SingleVariation(value).Build()
}

// makeUninitializedClientWithOverrides builds a client whose data system can never obtain
// LaunchDarkly data, with the given override source contents.
func makeUninitializedClientWithOverrides(
	t *testing.T,
	source *sharedtest.TestOverrideSource,
	events ldevents.EventProcessor,
) *LDClient {
	t.Helper()
	config := Config{
		Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
		DataSystem: ldcomponents.DataSystem().Custom().
			Synchronizers(newHangingSynchronizer()).
			Overrides(source),
	}
	if events == nil {
		config.Events = ldcomponents.NoEvents()
	} else {
		config.Events = mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: events}
	}
	client, _ := MakeCustomClient(testSdkKey, config, time.Duration(0))
	require.NotNil(t, client)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestOverrideIsServedWhenClientIsNotInitialized(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, nil)

	require.False(t, client.Initialized())

	value, detail, err := client.BoolVariationDetail("overridden-flag", evalTestUser, false)
	require.NoError(t, err)
	assert.True(t, value)
	assert.Equal(t, ldreason.EvalReasonOff, detail.Reason.GetKind())
	assert.True(t, detail.Reason.IsOverride())
}

func TestNonOverriddenFlagStillShortCircuitsWhenClientIsNotInitialized(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, nil)

	value, detail, err := client.BoolVariationDetail("other-flag", evalTestUser, false)
	assert.Equal(t, ErrClientNotInitialized, err)
	assert.False(t, value)
	assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorClientNotReady), detail.Reason)
}

func TestOverrideRemovalRestoresShortCircuit(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, nil)

	value, err := client.BoolVariation("overridden-flag", evalTestUser, false)
	require.NoError(t, err)
	require.True(t, value)

	source.SetOverrides(nil)

	value, err = client.BoolVariation("overridden-flag", evalTestUser, false)
	assert.Equal(t, ErrClientNotInitialized, err)
	assert.False(t, value)
}

func TestAllFlagsStateContainsOnlyOverridesWhenClientIsNotInitialized(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, nil)

	state := client.AllFlagsState(evalTestUser)
	assert.True(t, state.IsValid())
	values := state.ToValuesMap()
	require.Len(t, values, 1)
	assert.Equal(t, ldvalue.Bool(true), values["overridden-flag"])
}

func TestAllFlagsStateIsInvalidWhenNotInitializedAndOverrideLayerIsEmpty(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(nil)
	client := makeUninitializedClientWithOverrides(t, source, nil)

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)
}

func TestOverrideEvaluationEventsCarryOverrideMarker(t *testing.T) {
	events := &mocks.CapturingEventProcessor{}
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, events)

	_, err := client.BoolVariation("overridden-flag", evalTestUser, false)
	require.NoError(t, err)

	var evalData []ldevents.EvaluationData
	for _, e := range events.Events {
		if ed, ok := e.(ldevents.EvaluationData); ok {
			evalData = append(evalData, ed)
		}
	}
	require.Len(t, evalData, 1)
	assert.True(t, evalData[0].IsOverride)
	assert.Equal(t, "overridden-flag", evalData[0].Key)
}

func TestFlagTrackerIsNotifiedOfOverrideChanges(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(nil)
	client := makeUninitializedClientWithOverrides(t, source, nil)

	listener := client.GetFlagTracker().AddFlagChangeListener()

	source.SetOverrides(overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))

	select {
	case event := <-listener:
		assert.Equal(t, "overridden-flag", event.Key)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "timed out waiting for flag change event")
	}
}
