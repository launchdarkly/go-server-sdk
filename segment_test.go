package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	userKey := "foo"

	user := User{
		Key: &userKey,
	}

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

	userKey := "foo"

	user := User{
		Key: &userKey,
	}

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

	userKey := "foo"

	user := User{
		Key: &userKey,
	}

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestMatchingRuleWithFullRollout(t *testing.T) {
	rules := []SegmentRule{
		SegmentRule{
			Clauses: []Clause{Clause{
				Attribute: "email",
				Op:        operatorIn,
				Values:    []interface{}{"test@example.com"},
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

	userKey := "foo"
	userEmail := "test@example.com"

	user := User{
		Key:   &userKey,
		Email: &userEmail,
	}

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestMatchingRuleWithZeroRollout(t *testing.T) {
	rules := []SegmentRule{
		SegmentRule{
			Clauses: []Clause{Clause{
				Attribute: "email",
				Op:        operatorIn,
				Values:    []interface{}{"test@example.com"},
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

	userKey := "foo"
	userEmail := "test@example.com"

	user := User{
		Key:   &userKey,
		Email: &userEmail,
	}

	containsUser := segment.containsUser(user)
	assert.False(t, containsUser, "Segment %+v should not contain user %+v", segment, user)
}

func TestMatchingRuleWithMultipleClauses(t *testing.T) {
	rules := []SegmentRule{
		SegmentRule{
			Clauses: []Clause{
				Clause{
					Attribute: "email",
					Op:        operatorIn,
					Values:    []interface{}{"test@example.com"},
					Negate:    false,
				},
				Clause{
					Attribute: "name",
					Op:        operatorIn,
					Values:    []interface{}{"bob"},
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

	userKey := "foo"
	userEmail := "test@example.com"
	userName := "bob"

	user := User{
		Key:   &userKey,
		Email: &userEmail,
		Name:  &userName,
	}

	containsUser := segment.containsUser(user)
	assert.True(t, containsUser, "Segment %+v should contain user %+v", segment, user)
}

func TestNonMatchingRuleWithMultipleClauses(t *testing.T) {
	rules := []SegmentRule{
		SegmentRule{
			Clauses: []Clause{
				Clause{
					Attribute: "email",
					Op:        operatorIn,
					Values:    []interface{}{"test@example.com"},
					Negate:    false,
				},
				Clause{
					Attribute: "name",
					Op:        operatorIn,
					Values:    []interface{}{"bill"},
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

	userKey := "foo"
	userEmail := "test@example.com"
	userName := "bob"

	user := User{
		Key:   &userKey,
		Email: &userEmail,
		Name:  &userName,
	}

	containsUser := segment.containsUser(user)
	assert.False(t, containsUser, "Segment %+v should not contain user %+v", segment, user)
}
