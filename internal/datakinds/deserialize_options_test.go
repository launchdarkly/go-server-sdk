package datakinds

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/launchdarkly/go-jsonstream/v3/jreader"
	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// An "in" clause with this many values gets a lookup set, so the release removes its Values list.
const manyClauseValuesJSON = `["a","b","c","d","e","f","g","h"]`

const flagWithManyClauseValuesJSON = `{"key":"flagkey","version":1,"rules":[{"id":"r","clauses":[` +
	`{"attribute":"key","op":"in","values":` + manyClauseValuesJSON + `}]}]}`

const segmentWithManyClauseValuesJSON = `{"key":"segmentkey","version":1,"rules":[{"clauses":[` +
	`{"attribute":"key","op":"in","values":` + manyClauseValuesJSON + `}]}]}`

func manyClauseValues() []ldvalue.Value {
	return []ldvalue.Value{
		ldvalue.String("a"), ldvalue.String("b"), ldvalue.String("c"), ldvalue.String("d"),
		ldvalue.String("e"), ldvalue.String("f"), ldvalue.String("g"), ldvalue.String("h"),
	}
}

func firstClause(t *testing.T, item ldstoretypes.ItemDescriptor) *ldmodel.Clause {
	require.NotNil(t, item.Item)
	switch i := item.Item.(type) {
	case *ldmodel.FeatureFlag:
		return &i.Rules[0].Clauses[0]
	case *ldmodel.Segment:
		return &i.Rules[0].Clauses[0]
	}
	require.Fail(t, "unexpected item type")
	return nil
}

func TestDeserializeOptions(t *testing.T) {
	kinds := []struct {
		kind DataKindInternal
		json string
	}{
		{Features, flagWithManyClauseValuesJSON},
		{Segments, segmentWithManyClauseValuesJSON},
	}
	for _, k := range kinds {
		t.Run(k.kind.GetName(), func(t *testing.T) {
			deserializers := map[string]func(DeserializeOptions) (ldstoretypes.ItemDescriptor, error){
				"bytes": func(opts DeserializeOptions) (ldstoretypes.ItemDescriptor, error) {
					return k.kind.DeserializeWithOptions([]byte(k.json), opts)
				},
				"reader": func(opts DeserializeOptions) (ldstoretypes.ItemDescriptor, error) {
					r := jreader.NewReader([]byte(k.json))
					return k.kind.DeserializeFromJSONReaderWithOptions(&r, opts)
				},
				"Deserialize function": func(opts DeserializeOptions) (ldstoretypes.ItemDescriptor, error) {
					return Deserialize(k.kind, []byte(k.json), opts)
				},
			}
			for name, deserialize := range deserializers {
				t.Run(name, func(t *testing.T) {
					released, err := deserialize(NewDeserializeOptions(true))
					require.NoError(t, err)
					clause := firstClause(t, released)
					assert.Nil(t, clause.Values)
					assert.Equal(t, manyClauseValues(), slices.Collect(clause.AllValues()))

					kept, err := deserialize(DeserializeOptions{})
					require.NoError(t, err)
					assert.Equal(t, manyClauseValues(), firstClause(t, kept).Values)
					assert.JSONEq(t, string(k.kind.Serialize(kept)), string(k.kind.Serialize(released)))
				})
			}
		})
	}

	t.Run("default methods keep values", func(t *testing.T) {
		item, err := Features.Deserialize([]byte(flagWithManyClauseValuesJSON))
		require.NoError(t, err)
		assert.Equal(t, manyClauseValues(), firstClause(t, item).Values)

		r := jreader.NewReader([]byte(flagWithManyClauseValuesJSON))
		item, err = Features.DeserializeFromJSONReader(&r)
		require.NoError(t, err)
		assert.Equal(t, manyClauseValues(), firstClause(t, item).Values)
	})

	t.Run("deleted item", func(t *testing.T) {
		item, err := Features.DeserializeWithOptions([]byte(`{"key":"flagkey","version":2,"deleted":true}`),
			NewDeserializeOptions(true))
		require.NoError(t, err)
		assert.Equal(t, ldstoretypes.ItemDescriptor{Version: 2, Item: nil}, item)
	})
}

type otherDataKind struct{}

func (otherDataKind) GetName() string                              { return "other" }
func (otherDataKind) Serialize(ldstoretypes.ItemDescriptor) []byte { return nil }
func (otherDataKind) Deserialize([]byte) (ldstoretypes.ItemDescriptor, error) {
	return ldstoretypes.ItemDescriptor{Version: 3, Item: "other"}, nil
}

func TestDeserializeUsesOtherDataKind(t *testing.T) {
	item, err := Deserialize(otherDataKind{}, []byte(`{}`), NewDeserializeOptions(true))

	require.NoError(t, err)
	assert.Equal(t, ldstoretypes.ItemDescriptor{Version: 3, Item: "other"}, item)
}
