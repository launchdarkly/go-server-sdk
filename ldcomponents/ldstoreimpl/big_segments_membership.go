package ldstoreimpl

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// NewBigSegmentMembershipFromSegmentRefs creates a BigSegmentMembership based on the specified
// lists of included and excluded segment references. This method is intended to be used by Big
// Segment store implementations; application code does not need to use it.
//
// As described in interfaces.BigSegmentMembership, a segmentRef is not the same as the key
// property in the segment data model; it includes the key but also versioning information that the
// SDK will provide. The store implementation should not be concerned with the format of this.
//
// The returned object's CheckMembership method will return ldvalue.NewOptionalBool(true) for any
// segmentRef that is in the included list, ldvalue.NewOptionalBool(false) for any segmentRef that
// is in the excluded list and *not* also in the included list, and ldvalue.OptionalBool{}
// (undefined) for all others.
//
// The exact implementation type of the returned value may vary, to provide the most efficient
// representation of the data.
func NewBigSegmentMembershipFromSegmentRefs(
	includedSegmentRefs []string,
	excludedSegmentRefs []string,
) interfaces.BigSegmentMembership {
	if len(includedSegmentRefs) == 0 && len(excludedSegmentRefs) == 0 {
		return bigSegmentMembershipMapImpl(nil)
	}
	if len(includedSegmentRefs) == 1 && len(excludedSegmentRefs) == 0 {
		return bigSegmentMembershipSingleInclude(includedSegmentRefs[0])
	}
	if len(includedSegmentRefs) == 0 && len(excludedSegmentRefs) == 1 {
		return bigSegmentMembershipSingleExclude(excludedSegmentRefs[0])
	}
	ret := make(bigSegmentMembershipMapImpl, len(includedSegmentRefs)+len(excludedSegmentRefs))
	for _, exc := range excludedSegmentRefs {
		ret[exc] = false
	}
	for _, inc := range includedSegmentRefs { // includes override excludes
		ret[inc] = true
	}
	return ret
}

// This is the standard internal implementation of BigSegmentMembership. The map contains a true
// value for included keys and a false value for excluded keys that are not also included (inclusions
// override exclusions). If there are no keys at all, we store nil instead of allocating an empty map.
//
// Using a type that is simply a rename of a map is an efficient way to implement an interface, because
// no additional data structure besides the map needs to be allocated on the heap; the interface value
// contains only the type identifier and the map reference.
type bigSegmentMembershipMapImpl map[string]bool

// This is an alternate implementation of BigSegmentMembership used when there is only one key
// in the included list, and no keys in the excluded list, so there's no need to allocate a map.
type bigSegmentMembershipSingleInclude string

// This is an alternate implementation of BigSegmentMembership used when there is only one key
// in the excluded list, and no keys in the included list, so there's no need to allocate a map.
type bigSegmentMembershipSingleExclude string

func (u bigSegmentMembershipMapImpl) CheckMembership(segmentRef string) ldvalue.OptionalBool {
	value, found := u[segmentRef]
	if found {
		return ldvalue.NewOptionalBool(value)
	}
	return ldvalue.OptionalBool{}
}

func (u bigSegmentMembershipSingleInclude) CheckMembership(segmentRef string) ldvalue.OptionalBool {
	if segmentRef == string(u) {
		return ldvalue.NewOptionalBool(true)
	}
	return ldvalue.OptionalBool{}
}

func (u bigSegmentMembershipSingleExclude) CheckMembership(segmentRef string) ldvalue.OptionalBool {
	if segmentRef == string(u) {
		return ldvalue.NewOptionalBool(false)
	}
	return ldvalue.OptionalBool{}
}
