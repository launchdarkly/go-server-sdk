package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

var (
	max_weight = 100000
	min_weight = 0
)

func TestExplicitIncludeUser(t *testing.T) {
	segment := Segment{
		Key:      "test",
		Included: []string{"foo"},
		Excluded: nil,
		Salt:     "abcdef",
		Rules:    nil,
		Version:  1,
		Deleted:  false,
	}
	user := NewUser("foo")

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestExplicitExcludeUser(t *testing.T) {
	segment := Segment{
		Key:      "test",
		Included: nil,
		Excluded: []string{"foo"},
		Salt:     "abcdef",
		Rules:    nil,
		Version:  1,
		Deleted:  false,
	}
	user := NewUser("foo")

	containsUser := segment.containsUser(user)
	assert.False(t, containsUser, "Segment %+v should not contain user %+v", segment, user)
}

func TestExplicitIncludeHasPrecedence(t *testing.T) {
	segment := Segment{
		Key:      "test",
		Included: []string{"foo"},
		Excluded: []string{"foo"},
		Salt:     "abcdef",
		Rules:    nil,
		Version:  1,
		Deleted:  false,
	}
	user := NewUser("foo")

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestMatchingRuleWithFullRollout(t *testing.T) {
	rules := []segmentRule{
		segmentRule{
			Clauses: []clause{clause{
				Attribute: "email",
				Op:        operatorIn,
				Values:    []ldvalue.Value{ldvalue.String("test@example.com")},
				Negate:    false,
			}},
			Weight:   &max_weight,
			BucketBy: nil,
		},
	}

	segment := Segment{
		Key:      "test",
		Included: nil,
		Excluded: nil,
		Salt:     "abcdef",
		Rules:    rules,
		Version:  1,
		Deleted:  false,
	}

	user := NewUserBuilder("foo").Email("test@example.com").Build()

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestMatchingRuleWithZeroRollout(t *testing.T) {
	rules := []segmentRule{
		segmentRule{
			Clauses: []clause{clause{
				Attribute: "email",
				Op:        operatorIn,
				Values:    []ldvalue.Value{ldvalue.String("test@example.com")},
				Negate:    false,
			}},
			Weight:   &min_weight,
			BucketBy: nil,
		},
	}

	segment := Segment{
		Key:      "test",
		Included: nil,
		Excluded: nil,
		Salt:     "abcdef",
		Rules:    rules,
		Version:  1,
		Deleted:  false,
	}

	user := NewUserBuilder("foo").Email("test@example.com").Build()

	containsUser := segment.containsUser(user)
	assert.False(t, containsUser, "Segment %+v should not contain user %+v", segment, user)
}

func TestMatchingRuleWithMultipleClauses(t *testing.T) {
	rules := []segmentRule{
		segmentRule{
			Clauses: []clause{
				clause{
					Attribute: "email",
					Op:        operatorIn,
					Values:    []ldvalue.Value{ldvalue.String("test@example.com")},
				},
				clause{
					Attribute: "name",
					Op:        operatorIn,
					Values:    []ldvalue.Value{ldvalue.String("bob")},
				},
			},
			Weight:   nil,
			BucketBy: nil,
		},
	}

	segment := Segment{
		Key:      "test",
		Included: nil,
		Excluded: nil,
		Salt:     "abcdef",
		Rules:    rules,
		Version:  1,
		Deleted:  false,
	}

	user := NewUserBuilder("foo").Email("test@example.com").Name("bob").Build()

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestNonMatchingRuleWithMultipleClauses(t *testing.T) {
	rules := []segmentRule{
		segmentRule{
			Clauses: []clause{clause{
				Attribute: "email",
				Op:        operatorIn,
				Values:    []ldvalue.Value{ldvalue.String("test@example.com")},
				Negate:    false,
			},
				clause{
					Attribute: "name",
					Op:        operatorIn,
					Values:    []ldvalue.Value{ldvalue.String("bill")},
				},
			},
			Weight:   nil,
			BucketBy: nil,
		},
	}

	segment := Segment{
		Key:      "test",
		Included: nil,
		Excluded: nil,
		Salt:     "abcdef",
		Rules:    rules,
		Version:  1,
		Deleted:  false,
	}

	user := NewUserBuilder("foo").Email("test@example.com").Name("bob").Build()

	containsUser := segment.containsUser(user)
	assert.False(t, containsUser, "Segment %+v should not contain user %+v", segment, user)
}
