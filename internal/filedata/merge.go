package filedata

import (
	"fmt"

	"github.com/launchdarkly/go-server-sdk/v7/subsystems/ldstoretypes"
)

// DuplicateKeysHandling determines what happens when the same flag or segment key appears
// in more than one document.
//
// The values match the public option types in the packages that expose this behavior, so
// those types can be converted to this one directly.
type DuplicateKeysHandling string

const (
	// DuplicateKeysFail means a duplicated key causes the merge to fail.
	DuplicateKeysFail DuplicateKeysHandling = "fail"
	// DuplicateKeysIgnoreAllButFirst means only the first occurrence of a duplicated key is
	// used, in the order the documents were given.
	DuplicateKeysIgnoreAllButFirst DuplicateKeysHandling = "ignore"
)

// MergeResult holds the merged items from one or more documents. Item values are
// *ldmodel.FeatureFlag or *ldmodel.Segment.
//
// Ordering is deterministic at document granularity only: all of one document's items
// precede the next document's, matching the order the documents were given, but the
// relative order of items within a single document is unspecified. Consumers key items by
// their Key and must not rely on within-document ordering.
type MergeResult struct {
	Flags    []ldstoretypes.KeyedItemDescriptor
	Segments []ldstoretypes.KeyedItemDescriptor
	// Documents holds, for each input document in order, the number of entries the merge kept
	// from it. An entry dropped by the duplicate-key handling is not counted.
	Documents []DocumentSummary
	// Files is set by the Reloader. It describes each configured file in order.
	Files []FileSummary
}

// DocumentSummary counts the entries the merge kept from one document.
type DocumentSummary struct {
	Flags    int
	Segments int
}

// FileSummary describes one configured file after a reload.
type FileSummary struct {
	Path string
	// Present is false when the file does not exist and missing files are skipped.
	Present  bool
	Flags    int
	Segments int
}

type itemCategory string

const (
	flagCategory    itemCategory = "flag"
	segmentCategory itemCategory = "segment"
)

// Merge combines the items of the given documents, expanding flag-value entries into full
// flag definitions and applying the given duplicate-key handling. An unrecognized
// DuplicateKeysHandling value behaves as DuplicateKeysFail.
func Merge(duplicateKeysHandling DuplicateKeysHandling, docs ...Document) (MergeResult, error) {
	var result MergeResult
	seenKeys := map[itemCategory]map[string]bool{
		flagCategory:    {},
		segmentCategory: {},
	}

	// insert adds the entry unless the key was already seen. It reports whether it added it.
	insert := func(
		items *[]ldstoretypes.KeyedItemDescriptor,
		category itemCategory,
		key string,
		data ldstoretypes.ItemDescriptor,
	) (bool, error) {
		if seenKeys[category][key] {
			switch duplicateKeysHandling {
			case DuplicateKeysIgnoreAllButFirst:
				return false, nil
			default:
				return false, fmt.Errorf("%s '%s' is specified by multiple files", category, key)
			}
		}
		*items = append(*items, ldstoretypes.KeyedItemDescriptor{Key: key, Item: data})
		seenKeys[category][key] = true
		return true, nil
	}

	result.Documents = make([]DocumentSummary, len(docs))
	for i, d := range docs {
		if d.Flags != nil {
			for key, f := range *d.Flags {
				data := ldstoretypes.ItemDescriptor{Version: f.Version, Item: &f}
				added, err := insert(&result.Flags, flagCategory, key, data)
				if err != nil {
					return MergeResult{}, err
				}
				if added {
					result.Documents[i].Flags++
				}
			}
		}
		if d.FlagValues != nil {
			for key, value := range *d.FlagValues {
				flag := MakeFlagWithValue(key, value)
				data := ldstoretypes.ItemDescriptor{Version: flag.Version, Item: flag}
				added, err := insert(&result.Flags, flagCategory, key, data)
				if err != nil {
					return MergeResult{}, err
				}
				if added {
					result.Documents[i].Flags++
				}
			}
		}
		if d.Segments != nil {
			for key, s := range *d.Segments {
				data := ldstoretypes.ItemDescriptor{Version: s.Version, Item: &s}
				added, err := insert(&result.Segments, segmentCategory, key, data)
				if err != nil {
					return MergeResult{}, err
				}
				if added {
					result.Documents[i].Segments++
				}
			}
		}
	}
	return result, nil
}
