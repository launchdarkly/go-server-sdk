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

// MergeResult holds the merged items from one or more documents. Items appear in the order
// they were first encountered, processing documents in the order they were given. Item
// values are *ldmodel.FeatureFlag or *ldmodel.Segment.
type MergeResult struct {
	Flags    []ldstoretypes.KeyedItemDescriptor
	Segments []ldstoretypes.KeyedItemDescriptor
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

	insert := func(
		items *[]ldstoretypes.KeyedItemDescriptor,
		category itemCategory,
		key string,
		data ldstoretypes.ItemDescriptor,
	) error {
		if seenKeys[category][key] {
			switch duplicateKeysHandling {
			case DuplicateKeysIgnoreAllButFirst:
				return nil
			default:
				return fmt.Errorf("%s '%s' is specified by multiple files", category, key)
			}
		}
		*items = append(*items, ldstoretypes.KeyedItemDescriptor{Key: key, Item: data})
		seenKeys[category][key] = true
		return nil
	}

	for _, d := range docs {
		if d.Flags != nil {
			for key, f := range *d.Flags {
				data := ldstoretypes.ItemDescriptor{Version: f.Version, Item: &f}
				if err := insert(&result.Flags, flagCategory, key, data); err != nil {
					return MergeResult{}, err
				}
			}
		}
		if d.FlagValues != nil {
			for key, value := range *d.FlagValues {
				flag := MakeFlagWithValue(key, value)
				data := ldstoretypes.ItemDescriptor{Version: flag.Version, Item: flag}
				if err := insert(&result.Flags, flagCategory, key, data); err != nil {
					return MergeResult{}, err
				}
			}
		}
		if d.Segments != nil {
			for key, s := range *d.Segments {
				data := ldstoretypes.ItemDescriptor{Version: s.Version, Item: &s}
				if err := insert(&result.Segments, segmentCategory, key, data); err != nil {
					return MergeResult{}, err
				}
			}
		}
	}
	return result, nil
}
