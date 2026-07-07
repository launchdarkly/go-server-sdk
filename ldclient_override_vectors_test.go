package ldclient

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/launchdarkly/go-sdk-common/v3/ldcontext"
	"github.com/launchdarkly/go-sdk-common/v3/ldlogtest"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	ldevents "github.com/launchdarkly/go-sdk-events/v3"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldbuilders"
	"github.com/launchdarkly/go-server-sdk/v7/internal/datakinds"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest"
	"github.com/launchdarkly/go-server-sdk/v7/internal/sharedtest/mocks"
	"github.com/launchdarkly/go-server-sdk/v7/ldcomponents"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoreimpl"
	st "github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test runs the test vectors published with the OVERRIDE spec (copied verbatim from
// sdk-specs/specs/OVERRIDE-sdk-flag-overrides/test-vectors/vectors.json). Each vector sets up
// LaunchDarkly data, an override layer, and an initialization state, evaluates one flag
// through the full client stack, and checks the result, reason, and per-evaluation summary
// contribution.
const overrideVectorsPath = "testdata/override-vectors/vectors.json"

// The vectors' semantics are versioned; a schema change means this runner needs review.
const supportedOverrideVectorSchema = "0.1.0"

type overrideVectorFile struct {
	SchemaVersion string           `json:"schemaVersion"`
	Vectors       []overrideVector `json:"vectors"`
}

type overrideVector struct {
	Description      string `json:"description"`
	Group            string `json:"group"`
	LaunchDarklyData struct {
		Initialized bool                       `json:"initialized"`
		Flags       map[string]json.RawMessage `json:"flags"`
		Segments    map[string]json.RawMessage `json:"segments"`
	} `json:"launchDarklyData"`
	Overrides struct {
		Flags      map[string]json.RawMessage `json:"flags"`
		FlagValues map[string]ldvalue.Value   `json:"flagValues"`
		Segments   map[string]json.RawMessage `json:"segments"`
	} `json:"overrides"`
	Evaluate struct {
		FlagKey      string          `json:"flagKey"`
		Context      json.RawMessage `json:"context"`
		DefaultValue ldvalue.Value   `json:"defaultValue"`
	} `json:"evaluate"`
	Expect struct {
		Value           ldvalue.Value            `json:"value"`
		VariationIndex  ldvalue.Value            `json:"variationIndex"`
		Reason          map[string]ldvalue.Value `json:"reason"`
		SummaryOverride ldvalue.Value            `json:"summaryOverride"`
	} `json:"expect"`
}

// vectorInitializer supplies the vector's LaunchDarkly data as a full-transfer basis with a
// defined selector, which is what makes the client report full data availability.
type vectorInitializer struct {
	changeSet *subsystems.ChangeSet
}

func (v *vectorInitializer) Name() string { return "VectorInitializer" }

func (v *vectorInitializer) Fetch(ds subsystems.DataSelector, ctx context.Context) (*subsystems.Basis, bool, error) {
	return &subsystems.Basis{ChangeSet: *v.changeSet, Persist: false}, false, nil
}

func (v *vectorInitializer) Build(subsystems.ClientContext) (subsystems.DataInitializer, error) {
	return v, nil
}

func deserializeVectorItems(
	t *testing.T,
	kind st.DataKind,
	raw map[string]json.RawMessage,
) []st.KeyedItemDescriptor {
	t.Helper()
	var items []st.KeyedItemDescriptor
	for key, data := range raw {
		item, err := kind.Deserialize(data)
		require.NoError(t, err, "failed to parse %s %q", kind.GetName(), key)
		items = append(items, st.KeyedItemDescriptor{Key: key, Item: item})
	}
	return items
}

func (v *overrideVector) launchDarklyCollections(t *testing.T) []st.Collection {
	t.Helper()
	return []st.Collection{
		{Kind: ldstoreimpl.Features(), Items: deserializeVectorItems(t, datakinds.Features, v.LaunchDarklyData.Flags)},
		{Kind: ldstoreimpl.Segments(), Items: deserializeVectorItems(t, datakinds.Segments, v.LaunchDarklyData.Segments)},
	}
}

func (v *overrideVector) overrideCollections(t *testing.T) []st.Collection {
	t.Helper()
	flagItems := deserializeVectorItems(t, datakinds.Features, v.Overrides.Flags)
	for key, value := range v.Overrides.FlagValues {
		flag := ldbuilders.NewFlagBuilder(key).SingleVariation(value).Build()
		flagItems = append(flagItems, st.KeyedItemDescriptor{
			Key: key, Item: sharedtest.FlagDescriptor(flag),
		})
	}
	return []st.Collection{
		{Kind: ldstoreimpl.Features(), Items: flagItems},
		{Kind: ldstoreimpl.Segments(), Items: deserializeVectorItems(t, datakinds.Segments, v.Overrides.Segments)},
	}
}

func TestOverrideSpecVectors(t *testing.T) {
	data, err := os.ReadFile(overrideVectorsPath)
	require.NoError(t, err)
	var file overrideVectorFile
	require.NoError(t, json.Unmarshal(data, &file))
	require.Equal(t, supportedOverrideVectorSchema, file.SchemaVersion,
		"the vendored vectors changed schema; review this runner against the new schema before updating")
	require.NotEmpty(t, file.Vectors)

	for _, vector := range file.Vectors {
		t.Run(vector.Group+": "+vector.Description, func(t *testing.T) {
			runOverrideVector(t, vector)
		})
	}
}

func runOverrideVector(t *testing.T, vector overrideVector) {
	events := &mocks.CapturingEventProcessor{}
	source := sharedtest.NewTestOverrideSource(vector.overrideCollections(t))

	dataSystem := ldcomponents.DataSystem().Custom().Overrides(source)
	if vector.LaunchDarklyData.Initialized {
		intent := subsystems.ServerIntent{Payload: subsystems.Payload{
			Target: 1,
			Code:   subsystems.IntentTransferFull,
			Reason: "payload-missing",
		}}
		changeSet, err := subsystems.NewChangeSetFromCollections(intent,
			subsystems.NewSelector("vector-state", 1), vector.launchDarklyCollections(t))
		require.NoError(t, err)
		dataSystem = dataSystem.Initializers(&vectorInitializer{changeSet: changeSet})
	} else {
		// With no sources at all the client would consider cached data available rather
		// than applying its not-initialized handling, so configure a synchronizer that
		// never delivers anything.
		dataSystem = dataSystem.Synchronizers(newHangingSynchronizer())
	}

	config := Config{
		Logging:    ldcomponents.Logging().Loggers(ldlogtest.NewMockLog().Loggers),
		Events:     mocks.SingleComponentConfigurer[ldevents.EventProcessor]{Instance: events},
		DataSystem: dataSystem,
	}
	waitFor := 5 * time.Second
	if !vector.LaunchDarklyData.Initialized {
		waitFor = 0
	}
	client, _ := MakeCustomClient(testSdkKey, config, waitFor)
	require.NotNil(t, client)
	defer client.Close()

	var evalContext ldcontext.Context
	require.NoError(t, json.Unmarshal(vector.Evaluate.Context, &evalContext))

	// An error result (e.g. client not ready) is asserted through the detail below.
	_, detail, _ := client.JSONVariationDetail(vector.Evaluate.FlagKey, evalContext, vector.Evaluate.DefaultValue)

	assert.Equal(t, vector.Expect.Value, detail.Value, "value")

	if vector.Expect.VariationIndex.IsNull() {
		assert.False(t, detail.VariationIndex.IsDefined(), "variationIndex should be undefined")
	} else {
		assert.Equal(t, vector.Expect.VariationIndex.IntValue(), detail.VariationIndex.IntValue(), "variationIndex")
	}

	assertVectorReason(t, vector.Expect.Reason, detail.Reason)

	if !vector.Expect.SummaryOverride.IsNull() {
		var evalData []ldevents.EvaluationData
		for _, e := range events.Events {
			if ed, ok := e.(ldevents.EvaluationData); ok && ed.Key == vector.Evaluate.FlagKey {
				evalData = append(evalData, ed)
			}
		}
		require.Len(t, evalData, 1, "expected exactly one evaluation event for the flag")
		assert.Equal(t, vector.Expect.SummaryOverride.BoolValue(), evalData[0].IsOverride, "summaryOverride")
	}
}

// assertVectorReason compares the actual reason against only the fields present in the
// expected reason, per the vectors' comparison rules. isOverride collapses tri-state: an
// expected reason that omits it requires the actual reason to report false (never
// serialized) or omit it.
func assertVectorReason(t *testing.T, expected map[string]ldvalue.Value, actual interface{ MarshalJSON() ([]byte, error) }) {
	t.Helper()
	actualJSON, err := actual.MarshalJSON()
	require.NoError(t, err)
	var actualFields map[string]ldvalue.Value
	require.NoError(t, json.Unmarshal(actualJSON, &actualFields))

	for field, expectedValue := range expected {
		assert.Equal(t, expectedValue, actualFields[field], "reason field %q", field)
	}
	if _, present := expected["isOverride"]; !present {
		if actualValue, ok := actualFields["isOverride"]; ok {
			assert.False(t, actualValue.BoolValue(), "isOverride must be false or omitted")
		}
	}
}
