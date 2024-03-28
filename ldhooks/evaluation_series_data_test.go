package ldhooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanCreateEmptyData(t *testing.T) {
	empty := EmptyEvaluationSeriesData()
	assert.Empty(t, empty.data)
}

func TestCanCreateNewDataWithSetFields(t *testing.T) {
	withEntries := NewEvaluationSeriesBuilder(EmptyEvaluationSeriesData()).
		Set("A", "a").
		Set("B", "b").Build()

	assert.Len(t, withEntries.data, 2)

	aVal, aPresent := withEntries.Get("A")
	assert.True(t, aPresent)
	assert.Equal(t, "a", aVal)

	bVal, bPresent := withEntries.Get("B")
	assert.True(t, bPresent)
	assert.Equal(t, "b", bVal)
}

func TestCanAccessAMissingEntry(t *testing.T) {
	empty := EmptyEvaluationSeriesData()
	val, present := empty.Get("something")
	assert.Zero(t, val)
	assert.False(t, present)
}

func TestDataBuiltFromOtherDataDoesNotAffectOriginal(t *testing.T) {
	original := NewEvaluationSeriesBuilder(EmptyEvaluationSeriesData()).
		Set("A", "a").
		Set("B", "b").Build()

	derivative := NewEvaluationSeriesBuilder(original).
		Set("A", "AAA").Build()

	originalA, _ := original.Get("A")
	assert.Equal(t, "a", originalA)

	derivativeA, _ := derivative.Get("A")
	assert.Equal(t, "AAA", derivativeA)
}

func TestCanMergeDataFromMap(t *testing.T) {
	original := NewEvaluationSeriesBuilder(EmptyEvaluationSeriesData()).
		Set("A", "a").
		Set("B", "b").Build()

	merged := NewEvaluationSeriesBuilder(original).
		Merge(map[string]any{
			"A": "AAA",
			"C": "c",
		}).Build()

	originalA, _ := original.Get("A")
	assert.Equal(t, "a", originalA)

	originalC, originalCPresent := original.Get("C")
	assert.Zero(t, originalC)
	assert.False(t, originalCPresent)

	derivativeA, _ := merged.Get("A")
	assert.Equal(t, "AAA", derivativeA)

	derivativeC, _ := merged.Get("C")
	assert.Equal(t, "c", derivativeC)
}
