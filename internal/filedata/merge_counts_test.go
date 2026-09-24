package filedata

import (
	"testing"

	"github.com/launchdarkly/go-sdk-common/v3/ldvalue"
	"github.com/launchdarkly/go-server-sdk-evaluation/v3/ldmodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeCountsEntriesKeptFromEachDocument(t *testing.T) {
	first := Document{FlagValues: &map[string]ldvalue.Value{"a": ldvalue.Bool(true), "shared": ldvalue.Bool(true)}}
	segments := map[string]ldmodel.Segment{"seg": {Key: "seg"}}
	second := Document{
		FlagValues: &map[string]ldvalue.Value{"b": ldvalue.Bool(true), "shared": ldvalue.Bool(false)},
		Segments:   &segments,
	}

	result, err := Merge(DuplicateKeysIgnoreAllButFirst, first, second)
	require.NoError(t, err)
	require.Len(t, result.Documents, 2)
	assert.Equal(t, DocumentSummary{Flags: 2}, result.Documents[0])
	// The duplicate "shared" entry from the second document is dropped and not counted.
	assert.Equal(t, DocumentSummary{Flags: 1, Segments: 1}, result.Documents[1])
}
