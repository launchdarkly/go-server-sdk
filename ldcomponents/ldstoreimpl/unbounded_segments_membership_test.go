package ldstoreimpl

import (
	"testing"

	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"

	"github.com/stretchr/testify/assert"
)

func TestMembershipWithNilKeys(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys(nil, nil)
	assert.Equal(t, unboundedSegmentMembershipMapImpl(nil), m)
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key"))
}

func TestMembershipWithEmptySliceKeys(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys(nil, nil)
	assert.Equal(t, unboundedSegmentMembershipMapImpl(nil), m)
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key"))
}

func TestMembershipWithSingleIncludedKey(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys([]string{"key1"}, nil)
	assert.Equal(t, unboundedSegmentMembershipSingleInclude("key1"), m)
	assert.Equal(t, ldvalue.NewOptionalBool(true), m.CheckMembership("key1"))
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key2"))
}

func TestMembershipWithSingleExcludedKey(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys(nil, []string{"key1"})
	assert.Equal(t, unboundedSegmentMembershipSingleExclude("key1"), m)
	assert.Equal(t, ldvalue.NewOptionalBool(false), m.CheckMembership("key1"))
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key2"))
}

func TestMembershipWithMultipleIncludedKeys(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys([]string{"key1", "key2"}, nil)
	assert.Equal(t, unboundedSegmentMembershipMapImpl(map[string]bool{"key1": true, "key2": true}), m)
	assert.Equal(t, ldvalue.NewOptionalBool(true), m.CheckMembership("key1"))
	assert.Equal(t, ldvalue.NewOptionalBool(true), m.CheckMembership("key2"))
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key3"))
}

func TestMembershipWithMultipleExcludedKeys(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys(nil, []string{"key1", "key2"})
	assert.Equal(t, unboundedSegmentMembershipMapImpl(map[string]bool{"key1": false, "key2": false}), m)
	assert.Equal(t, ldvalue.NewOptionalBool(false), m.CheckMembership("key1"))
	assert.Equal(t, ldvalue.NewOptionalBool(false), m.CheckMembership("key2"))
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key3"))
}

func TestMembershipWithIncludedAndExcludedKeys(t *testing.T) {
	m := NewUnboundedSegmentMembershipFromKeys([]string{"key1", "key2"}, []string{"key2", "key3"})
	// key1 is included; key2 is included and excluded, therefore it's included; key3 is excluded
	assert.Equal(t, unboundedSegmentMembershipMapImpl(map[string]bool{"key1": true, "key2": true, "key3": false}), m)
	assert.Equal(t, ldvalue.NewOptionalBool(true), m.CheckMembership("key1"))
	assert.Equal(t, ldvalue.NewOptionalBool(true), m.CheckMembership("key2"))
	assert.Equal(t, ldvalue.NewOptionalBool(false), m.CheckMembership("key3"))
	assert.Equal(t, ldvalue.OptionalBool{}, m.CheckMembership("key4"))
}
