package ldclient

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/launchdarkly/go-sdk-common/v3/ldtime"
	"github.com/launchdarkly/go-server-sdk/v7/interfaces/flagstate"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
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
	return makeUninitializedClientWithOverridesAndLog(t, source, events, ldlogtest.NewMockLog())
}

func makeUninitializedClientWithOverridesAndLog(
	t *testing.T,
	source *sharedtest.TestOverrideSource,
	events ldevents.EventProcessor,
	mockLog *ldlogtest.MockLog,
) *LDClient {
	t.Helper()
	config := Config{
		Logging: ldcomponents.Logging().Loggers(mockLog.Loggers),
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
	assert.True(t, detail.Reason.IsOverrideAffected())
}

func TestNonOverriddenFlagStillShortCircuitsWhenClientIsNotInitialized(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, nil)

	value, detail, err := client.BoolVariationDetail("other-flag", evalTestUser, false)
	assert.Equal(t, ErrClientNotInitialized, err)
	assert.False(t, value)
	assert.Equal(t, ldreason.NewEvalReasonError(ldreason.EvalErrorClientNotReady), detail.Reason)
	assert.False(t, detail.Reason.IsOverrideAffected())
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

func TestAllFlagsStateOverridesOnlyWarningIsLoggedOnce(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	mockLog := ldlogtest.NewMockLog()
	client := makeUninitializedClientWithOverridesAndLog(t, source, nil, mockLog)

	require.True(t, client.AllFlagsState(evalTestUser).IsValid())
	require.True(t, client.AllFlagsState(evalTestUser).IsValid())

	var matching []string
	for _, line := range mockLog.GetOutput(ldlog.Warn) {
		if strings.Contains(line, "returning only flags from the override layer") {
			matching = append(matching, line)
		}
	}
	assert.Len(t, matching, 1)
}

func TestAllFlagsStateTurnsOffEventTrackingForOverrideAffectedFlags(t *testing.T) {
	debugUntil := ldtime.UnixMillisNow() + 100000
	plainTracked := ldbuilders.NewFlagBuilder("plain-tracked").Version(1).On(false).OffVariation(0).
		Variations(ldvalue.Bool(true)).TrackEvents(true).DebugEventsUntilDate(debugUntil).Build()
	dependentTracked := ldbuilders.NewFlagBuilder("dependent-tracked").Version(1).On(true).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).FallthroughVariation(1).
		AddPrerequisite("overridden-flag", 0).TrackEvents(true).DebugEventsUntilDate(debugUntil).Build()
	// The overridden flag is on and serves variation 0, so the dependent flag's prerequisite passes.
	overriddenTracked := ldbuilders.NewFlagBuilder("overridden-flag").Version(7).On(true).FallthroughVariation(0).
		Variations(ldvalue.Bool(true)).TrackEvents(true).DebugEventsUntilDate(debugUntil).Build()

	source := sharedtest.NewTestOverrideSource(overrideTestFlagData(overriddenTracked))
	client := makeInitializedClientWithOverrides(t, overrideTestFlagData(plainTracked, dependentTracked), source, nil)

	state := client.AllFlagsState(evalTestUser, flagstate.OptionWithReasons())
	require.True(t, state.IsValid())
	stateJSON, err := json.Marshal(state)
	require.NoError(t, err)
	flagsState := ldvalue.Parse(stateJSON).GetByKey("$flagsState")

	// Step 1: a flag with no override keeps its tracking fields.
	plain := flagsState.GetByKey("plain-tracked")
	assert.True(t, plain.GetByKey("trackEvents").BoolValue())
	assert.Equal(t, float64(debugUntil), plain.GetByKey("debugEventsUntilDate").Float64Value())

	// Step 2: the overridden flag and the flag that depends on it stay in the state with their
	// values and marked reasons, but with no tracking fields.
	for _, key := range []string{"overridden-flag", "dependent-tracked"} {
		entry := flagsState.GetByKey(key)
		assert.True(t, entry.GetByKey("reason").GetByKey("overrideAffected").BoolValue(), key)
		assert.Equal(t, ldvalue.Null(), entry.GetByKey("trackEvents"), key+" trackEvents")
		assert.Equal(t, ldvalue.Null(), entry.GetByKey("trackReason"), key+" trackReason")
		assert.Equal(t, ldvalue.Null(), entry.GetByKey("debugEventsUntilDate"), key+" debugEventsUntilDate")
	}
	assert.Equal(t, ldvalue.Bool(true), ldvalue.Parse(stateJSON).GetByKey("overridden-flag"))
	assert.Equal(t, ldvalue.Bool(true), ldvalue.Parse(stateJSON).GetByKey("dependent-tracked"))
}

func TestAllFlagsStateIsInvalidWhenNotInitializedAndOverrideLayerIsEmpty(t *testing.T) {
	source := sharedtest.NewTestOverrideSource(nil)
	client := makeUninitializedClientWithOverrides(t, source, nil)

	state := client.AllFlagsState(evalTestUser)
	assert.False(t, state.IsValid())
	assert.Len(t, state.ToValuesMap(), 0)
}

func TestOverrideEvaluationEventsCarryOverrideAffectedMarking(t *testing.T) {
	events := &mocks.CapturingEventProcessor{}
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.Bool(true))))
	client := makeUninitializedClientWithOverrides(t, source, events)

	_, err := client.BoolVariation("overridden-flag", evalTestUser, false)
	require.NoError(t, err)

	records := evaluationRecordsByKey(events)
	require.Len(t, records, 1)
	assert.True(t, records["overridden-flag"].OverrideAffected)
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

// evaluationRecordsByKey returns the evaluation records the client handed to the event
// processor, keyed by flag key. Each flag is expected to be evaluated at most once.
func evaluationRecordsByKey(events *mocks.CapturingEventProcessor) map[string]ldevents.EvaluationData {
	records := map[string]ldevents.EvaluationData{}
	for _, e := range events.Events {
		if ed, ok := e.(ldevents.EvaluationData); ok {
			records[ed.Key] = ed
		}
	}
	return records
}

// staticBasisInitializer supplies fixed LaunchDarkly data as a full-transfer basis with a
// selector, so the client reports full data availability once it has applied the data.
type staticBasisInitializer struct {
	changeSet *subsystems.ChangeSet
}

func (s *staticBasisInitializer) Name() string { return "StaticBasisInitializer" }

func (s *staticBasisInitializer) Fetch(
	ds subsystems.DataSelector,
	ctx context.Context,
) (*subsystems.Basis, bool, error) {
	return &subsystems.Basis{ChangeSet: *s.changeSet, Persist: false}, false, nil
}

func (s *staticBasisInitializer) Build(subsystems.ClientContext) (subsystems.DataInitializer, error) {
	return s, nil
}

// makeInitializedClientWithOverrides builds a client that has initialized with the given
// LaunchDarkly data, with the given override source contents and event processor.
func makeInitializedClientWithOverrides(
	t *testing.T,
	launchDarklyData []st.Collection,
	source *sharedtest.TestOverrideSource,
	events ldevents.EventProcessor,
) *LDClient {
	t.Helper()
	intent := subsystems.ServerIntent{Payload: subsystems.Payload{
		Target: 1,
		Code:   subsystems.IntentTransferFull,
		Reason: "payload-missing",
	}}
	changeSet, err := subsystems.NewChangeSetFromCollections(intent,
		subsystems.NewSelector("test-state", 1), launchDarklyData)
	require.NoError(t, err)
	config := Config{
		Logging: ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
		Events:  mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: events},
		DataSystem: ldcomponents.DataSystem().Custom().
			Initializers(&staticBasisInitializer{changeSet: changeSet}).
			Overrides(source),
	}
	client, err := MakeCustomClient(testSdkKey, config, 5*time.Second)
	require.NoError(t, err)
	require.NotNil(t, client)
	t.Cleanup(func() { _ = client.Close() })
	require.True(t, client.Initialized())
	return client
}

// The prerequisite tree used by the marking tests. Every flag requests individual events.
//
//	top-flag (LaunchDarkly) --> mid-flag (LaunchDarkly) --> leaf-flag (overridden)
//	                        --> plain-flag (LaunchDarkly)
//
// The LaunchDarkly copy of leaf-flag is off, so the chain passes only through the override.
const (
	treeTopKey   = "top-flag"
	treeMidKey   = "mid-flag"
	treeLeafKey  = "leaf-flag"
	treePlainKey = "plain-flag"
)

func trackedBoolFlag(key string) *ldbuilders.FlagBuilder {
	return ldbuilders.NewFlagBuilder(key).
		Variations(ldvalue.Bool(false), ldvalue.Bool(true)).
		OffVariation(0).
		FallthroughVariation(1).
		TrackEvents(true)
}

func prerequisiteTreeLaunchDarklyData() []st.Collection {
	return overrideTestFlagData(
		trackedBoolFlag(treeTopKey).On(true).
			AddPrerequisite(treeMidKey, 1).
			AddPrerequisite(treePlainKey, 1).
			Build(),
		trackedBoolFlag(treeMidKey).On(true).AddPrerequisite(treeLeafKey, 1).Build(),
		trackedBoolFlag(treePlainKey).On(true).Build(),
		trackedBoolFlag(treeLeafKey).On(false).Build(),
	)
}

func prerequisiteTreeOverrides() []st.Collection {
	return overrideTestFlagData(trackedBoolFlag(treeLeafKey).On(true).Build())
}

func TestOverriddenPrerequisiteMarksTheDependentEvaluationRecords(t *testing.T) {
	events := &mocks.CapturingEventProcessor{}
	source := sharedtest.NewTestOverrideSource(prerequisiteTreeOverrides())
	client := makeInitializedClientWithOverrides(t, prerequisiteTreeLaunchDarklyData(), source, events)

	value, detail, err := client.BoolVariationDetail(treeTopKey, evalTestUser, false)
	require.NoError(t, err)
	assert.True(t, value)
	assert.Equal(t, ldreason.EvalReasonFallthrough, detail.Reason.GetKind())
	assert.True(t, detail.Reason.IsOverrideAffected())

	records := evaluationRecordsByKey(events)
	require.Len(t, records, 4)

	// The top-level flag came from LaunchDarkly. Its record is marked because a definition
	// read during its evaluation came from the override store.
	assert.True(t, records[treeTopKey].OverrideAffected)
	assert.False(t, records[treeTopKey].PrereqOf.IsDefined())

	// The intermediate prerequisite also came from LaunchDarkly. Its own subtree read the
	// override, so its record is marked.
	assert.True(t, records[treeMidKey].OverrideAffected)
	assert.Equal(t, treeTopKey, records[treeMidKey].PrereqOf.StringValue())

	// The overridden leaf is marked directly.
	assert.True(t, records[treeLeafKey].OverrideAffected)
	assert.Equal(t, treeMidKey, records[treeLeafKey].PrereqOf.StringValue())

	// The sibling prerequisite read nothing from the override store, so it is not marked.
	assert.False(t, records[treePlainKey].OverrideAffected)
	assert.Equal(t, treeTopKey, records[treePlainKey].PrereqOf.StringValue())

	// Every flag requested individual events. The marking alone decides which records the
	// event processor keeps out of the individual event stream.
	for key, record := range records {
		assert.True(t, record.RequireFullEvent, key)
	}
}

// capturingEventSender keeps the analytics payloads the event processor sends.
type capturingEventSender struct {
	mu       sync.Mutex
	payloads [][]byte
}

func (c *capturingEventSender) SendEventData(
	kind ldevents.EventDataKind,
	data []byte,
	eventCount int,
) ldevents.EventSenderResult {
	if kind == ldevents.AnalyticsEventDataKind {
		c.mu.Lock()
		c.payloads = append(c.payloads, data)
		c.mu.Unlock()
	}
	return ldevents.EventSenderResult{Success: true}
}

func (c *capturingEventSender) outputEvents(t *testing.T) []ldvalue.Value {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var all []ldvalue.Value
	for _, payload := range c.payloads {
		var events []ldvalue.Value
		require.NoError(t, json.Unmarshal(payload, &events))
		all = append(all, events...)
	}
	return all
}

func TestOverrideAffectedEvaluationsAppearOnlyInSummaryOutput(t *testing.T) {
	sender := &capturingEventSender{}
	processor := ldevents.NewDefaultEventProcessor(ldevents.EventsConfiguration{
		Capacity:              1000,
		EventSender:           sender,
		FlushInterval:         time.Hour,
		Loggers:               ldlog.NewDisabledLoggers(),
		UserKeysCapacity:      1000,
		UserKeysFlushInterval: time.Hour,
	})
	source := sharedtest.NewTestOverrideSource(prerequisiteTreeOverrides())
	client := makeInitializedClientWithOverrides(t, prerequisiteTreeLaunchDarklyData(), source, processor)

	value, err := client.BoolVariation(treeTopKey, evalTestUser, false)
	require.NoError(t, err)
	require.True(t, value)
	require.True(t, client.FlushAndWait(5*time.Second))

	var featureEventKeys []string
	var summary ldvalue.Value
	for _, event := range sender.outputEvents(t) {
		switch event.GetByKey("kind").StringValue() {
		case ldevents.FeatureRequestEventKind:
			featureEventKeys = append(featureEventKeys, event.GetByKey("key").StringValue())
		case ldevents.SummaryEventKind:
			summary = event
		}
	}

	// Only the unaffected sibling produces an individual event, even though every flag in
	// the tree has event tracking on.
	assert.Equal(t, []string{treePlainKey}, featureEventKeys)

	require.False(t, summary.IsNull(), "expected a summary event")
	features := summary.GetByKey("features")
	for _, key := range []string{treeTopKey, treeMidKey, treeLeafKey, treePlainKey} {
		counters := features.GetByKey(key).GetByKey("counters")
		require.Equal(t, 1, counters.Count(), key)
		marker := counters.GetByIndex(0).GetByKey("overrideAffected")
		if key == treePlainKey {
			assert.True(t, marker.IsNull(), "%s: the marker is omitted for an unaffected counter", key)
		} else {
			assert.Equal(t, ldvalue.Bool(true), marker, "%s: the counter carries the marker", key)
		}
	}
}

func TestWrongTypeResultOfOverriddenFlagStaysMarked(t *testing.T) {
	events := &mocks.CapturingEventProcessor{}
	source := sharedtest.NewTestOverrideSource(
		overrideTestFlagData(singleValueFlag("overridden-flag", ldvalue.String("not-a-bool"))))
	client := makeUninitializedClientWithOverrides(t, source, events)

	value, detail, err := client.BoolVariationDetail("overridden-flag", evalTestUser, false)
	require.NoError(t, err)
	assert.False(t, value)
	assert.False(t, detail.VariationIndex.IsDefined())
	assert.Equal(t, ldreason.EvalReasonError, detail.Reason.GetKind())
	assert.Equal(t, ldreason.EvalErrorWrongType, detail.Reason.GetErrorKind())
	assert.True(t, detail.Reason.IsOverrideAffected())

	reasonJSON, err := json.Marshal(detail.Reason)
	require.NoError(t, err)
	assert.JSONEq(t, `{"kind":"ERROR","errorKind":"WRONG_TYPE","overrideAffected":true}`, string(reasonJSON))

	records := evaluationRecordsByKey(events)
	require.Len(t, records, 1)
	record := records["overridden-flag"]
	assert.True(t, record.OverrideAffected)
	assert.Equal(t, ldvalue.Bool(false), record.Value)
	assert.False(t, record.Variation.IsDefined())
}
