package ldstoreimpl

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk.v5/interfaces"
)

// NewUnboundedSegmentMembershipFromKeys creates an UnboundedSegmentMembership based on the specified
// lists of included and excluded keys. This method is intended to be used by unbounded segment store
// implementations; application code does not need to use it.
//
// The returned object's CheckMembership method will return ldvalue.NewOptionalBool(true) for any key
// that is in the included list, ldvalue.NewOptionalBool(false) for any key that is in the excluded
// list and *not* also in the included list, and ldvalue.OptionalBool{} (undefined) for all other keys.
//
// The exact implementation type of the returned value may vary, to provide the most efficient
// representation of the data.
func NewUnboundedSegmentMembershipFromKeys(
	includedKeys []string,
	excludedKeys []string,
) interfaces.UnboundedSegmentMembership {
	if len(includedKeys) == 0 && len(excludedKeys) == 0 {
		return unboundedSegmentMembershipMapImpl(nil)
	}
	if len(includedKeys) == 1 && len(excludedKeys) == 0 {
		return unboundedSegmentMembershipSingleInclude(includedKeys[0])
	}
	if len(includedKeys) == 0 && len(excludedKeys) == 1 {
		return unboundedSegmentMembershipSingleExclude(excludedKeys[0])
	}
	ret := make(unboundedSegmentMembershipMapImpl, len(includedKeys)+len(excludedKeys))
	for _, exc := range excludedKeys {
		ret[exc] = false
	}
	for _, inc := range includedKeys { // includes override excludes
		ret[inc] = true
	}
	return ret
}

// This is the standard internal implementation of UnboundedSegmentMembership. The map contains a true
// value for included keys and a false value for excluded keys that are not also included (inclusions
// override exclusions). If there are no keys at all, we store nil instead of allocating an empty map.
//
// Using a type that is simply a rename of a map is an efficient way to implement an interface, because
// no additional data structure besides the map needs to be allocated on the heap; the interface value
// contains only the type identifier and the map reference.
type unboundedSegmentMembershipMapImpl map[string]bool

// This is an alternate implementation of UnboundedSegmentMembership used when there is only one key
// in the included list, and no keys in the excluded list, so there's no need to allocate a map.
type unboundedSegmentMembershipSingleInclude string

// This is an alternate implementation of UnboundedSegmentMembership used when there is only one key
// in the excluded list, and no keys in the included list, so there's no need to allocate a map.
type unboundedSegmentMembershipSingleExclude string

func (u unboundedSegmentMembershipMapImpl) CheckMembership(segmentKey string) ldvalue.OptionalBool {
	value, found := u[segmentKey]
	if found {
		return ldvalue.NewOptionalBool(value)
	}
	return ldvalue.OptionalBool{}
}

func (u unboundedSegmentMembershipSingleInclude) CheckMembership(segmentKey string) ldvalue.OptionalBool {
	if segmentKey == string(u) {
		return ldvalue.NewOptionalBool(true)
	}
	return ldvalue.OptionalBool{}
}

func (u unboundedSegmentMembershipSingleExclude) CheckMembership(segmentKey string) ldvalue.OptionalBool {
	if segmentKey == string(u) {
		return ldvalue.NewOptionalBool(false)
	}
	return ldvalue.OptionalBool{}
}
