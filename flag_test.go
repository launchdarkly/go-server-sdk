package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

var flagUser = NewUser("x")
var emptyFeatureStore = NewInMemoryFeatureStore(nil)

func intPtr(n int) *int {
	return &n
}

func TestFlagReturnsOffVariationIfFlagIsOff(t *testing.T) {
	f := FeatureFlag{
		Key:          "feature",
		On:           false,
		OffVariation: intPtr(1),
		Fallthrough:  VariationOrRollout{Variation: intPtr(0)},
		Variations:   []interface{}{"fall", "off", "on"},
	}

	value, events := f.Evaluate(flagUser, emptyFeatureStore)
	assert.Equal(t, "off", value)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsNilIfFlagIsOffAndOffVariationIsUnspecified(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          false,
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	value, events := f.Evaluate(flagUser, emptyFeatureStore)
	assert.Equal(t, nil, value)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsOffVariationIfPrerequisiteIsNotFound(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []Prerequisite{Prerequisite{"feature1", 1}},
		Fallthrough:   VariationOrRollout{Variation: intPtr(0)},
		Variations:    []interface{}{"fall", "off", "on"},
	}

	value, events := f0.Evaluate(flagUser, emptyFeatureStore)
	assert.Equal(t, "off", value)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsOffVariationAndEventIfPrerequisiteIsNotMet(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []Prerequisite{Prerequisite{"feature1", 1}},
		Fallthrough:   VariationOrRollout{Variation: intPtr(0)},
		Variations:    []interface{}{"fall", "off", "on"},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           true,
		OffVariation: intPtr(1),
		Fallthrough:  VariationOrRollout{Variation: intPtr(0)},
		Variations:   []interface{}{"nogo", "go"},
		Version:      2,
	}
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Features, &f1)

	value, events := f0.Evaluate(flagUser, featureStore)
	assert.Equal(t, "off", value)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, "feature", e.Kind)
	assert.Equal(t, "nogo", e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestFlagReturnsFallthroughVariationAndEventIfPrerequisiteIsMetAndThereAreNoRules(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []Prerequisite{Prerequisite{"feature1", 1}},
		Fallthrough:   VariationOrRollout{Variation: intPtr(0)},
		Variations:    []interface{}{"fall", "off", "on"},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           true,
		OffVariation: intPtr(1),
		Fallthrough:  VariationOrRollout{Variation: intPtr(1)}, // this 1 matches the 1 in the prerequisites array
		Variations:   []interface{}{"nogo", "go"},
		Version:      2,
	}
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Features, &f1)

	value, events := f0.Evaluate(flagUser, featureStore)
	assert.Equal(t, "fall", value)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, "feature", e.Kind)
	assert.Equal(t, "go", e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestMultipleLevelsOfPrerequisiteProduceMultipleEvents(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []Prerequisite{Prerequisite{"feature1", 1}},
		Fallthrough:   VariationOrRollout{Variation: intPtr(0)},
		Variations:    []interface{}{"fall", "off", "on"},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:           "feature1",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []Prerequisite{Prerequisite{"feature2", 1}},
		Fallthrough:   VariationOrRollout{Variation: intPtr(1)}, // this 1 matches the 1 in the prerequisites array
		Variations:    []interface{}{"nogo", "go"},
		Version:       2,
	}
	f2 := FeatureFlag{
		Key:         "feature2",
		On:          true,
		Fallthrough: VariationOrRollout{Variation: intPtr(1)},
		Variations:  []interface{}{"nogo", "go"},
		Version:     3,
	}
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Features, &f1)
	featureStore.Upsert(Features, &f2)

	value, events := f0.Evaluate(flagUser, featureStore)
	assert.Equal(t, "fall", value)

	assert.Equal(t, 2, len(events))
	// events are generated recursively, so the deepest level of prerequisite appears first

	e0 := events[0]
	assert.Equal(t, f2.Key, e0.Key)
	assert.Equal(t, "feature", e0.Kind)
	assert.Equal(t, "go", e0.Value)
	assert.Equal(t, intPtr(f2.Version), e0.Version)
	assert.Equal(t, strPtr(f1.Key), e0.PrereqOf)

	e1 := events[1]
	assert.Equal(t, f1.Key, e1.Key)
	assert.Equal(t, "feature", e1.Kind)
	assert.Equal(t, "go", e1.Value)
	assert.Equal(t, intPtr(f1.Version), e1.Version)
	assert.Equal(t, strPtr(f0.Key), e1.PrereqOf)
}

func TestFlagMatchesUserFromTargets(t *testing.T) {
	f := FeatureFlag{
		Key:          "feature",
		On:           true,
		OffVariation: intPtr(1),
		Targets:      []Target{Target{[]string{"whoever", "userkey"}, 2}},
		Fallthrough:  VariationOrRollout{Variation: intPtr(0)},
		Variations:   []interface{}{"fall", "off", "on"},
	}
	user := NewUser("userkey")

	value, events := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, "on", value)
	assert.Equal(t, 0, len(events))
}

func TestFlagMatchesUserFromRules(t *testing.T) {
	clause := Clause{
		Attribute: "key",
		Op:        "in",
		Values:    []interface{}{"userkey"},
	}
	f := FeatureFlag{
		Key:          "feature",
		On:           true,
		OffVariation: intPtr(1),
		Rules: []Rule{
			Rule{
				Clauses:            []Clause{clause},
				VariationOrRollout: VariationOrRollout{Variation: intPtr(2)},
			},
		},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"fall", "off", "on"},
	}
	user := NewUser("userkey")

	value, events := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, "on", value)
	assert.Equal(t, 0, len(events))
}

func TestClauseCanMatchBuiltInAttribute(t *testing.T) {
	clause := Clause{
		Attribute: "name",
		Op:        "in",
		Values:    []interface{}{"Bob"},
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, true, value)
}

func TestClauseCanMatchCustomAttribute(t *testing.T) {
	clause := Clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []interface{}{4},
	}
	f := booleanFlagWithClause(clause)
	custom := map[string]interface{}{"legs": 4}
	user := User{Key: strPtr("key"), Custom: &custom}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, true, value)
}

func TestClauseReturnsFalseForMissingAttribute(t *testing.T) {
	clause := Clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []interface{}{4},
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, false, value)
}

func TestClauseCanBeNegated(t *testing.T) {
	clause := Clause{
		Attribute: "name",
		Op:        "in",
		Values:    []interface{}{"Bob"},
		Negate:    true,
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, false, value)
}

func TestClauseForMissingAttributeIsFalseEvenIfNegated(t *testing.T) {
	clause := Clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []interface{}{4},
		Negate:    true,
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, false, value)
}

func TestClauseWithUnknownOperatorDoesNotMatch(t *testing.T) {
	clause := Clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []interface{}{"Bob"},
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, false, value)
}

func TestClauseWithUnknownOperatorDoesNotStopSubsequentRuleFromMatching(t *testing.T) {
	badClause := Clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []interface{}{"Bob"},
	}
	badRule := Rule{Clauses: []Clause{badClause}, VariationOrRollout: VariationOrRollout{Variation: intPtr(1)}}
	goodClause := Clause{
		Attribute: "name",
		Op:        "in",
		Values:    []interface{}{"Bob"},
	}
	goodRule := Rule{Clauses: []Clause{goodClause}, VariationOrRollout: VariationOrRollout{Variation: intPtr(1)}}
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{badRule, goodRule},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{false, true},
	}
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, true, value)
}

func TestSegmentMatchClauseRetrievesSegmentFromStore(t *testing.T) {
	segment := Segment{
		Key:      "segkey",
		Included: []string{"foo"},
	}
	f := booleanFlagWithClause(Clause{Attribute: "", Op: "segmentMatch", Values: []interface{}{"segkey"}})
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Segments, &segment)
	user := NewUser("foo")

	value, _ := f.Evaluate(user, featureStore)
	assert.Equal(t, true, value)
}

func TestSegmentMatchClauseFallsThroughIfSegmentNotFound(t *testing.T) {
	f := booleanFlagWithClause(Clause{Attribute: "", Op: "segmentMatch", Values: []interface{}{"segkey"}})
	user := NewUser("foo")

	value, _ := f.Evaluate(user, emptyFeatureStore)
	assert.Equal(t, false, value)
}

func TestVariationIndexForUser(t *testing.T) {
	wv1 := WeightedVariation{Variation: 0, Weight: 60000.0}
	wv2 := WeightedVariation{Variation: 1, Weight: 40000.0}
	rollout := Rollout{Variations: []WeightedVariation{wv1, wv2}}
	rule := Rule{VariationOrRollout: VariationOrRollout{Rollout: &rollout}}

	userKey := "userKeyA"
	variationIndex := rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)

	userKey = "userKeyB"
	variationIndex = rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 1, *variationIndex)

	userKey = "userKeyC"
	variationIndex = rule.variationIndexForUser(User{Key: &userKey}, "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)
}

func TestBucketUserByKey(t *testing.T) {
	userKey := "userKeyA"
	user := User{Key: &userKey}
	bucket := bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.42157587, bucket, 0.0000001)

	userKey = "userKeyB"
	user = User{Key: &userKey}
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.6708485, bucket, 0.0000001)

	userKey = "userKeyC"
	user = User{Key: &userKey}
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.10343106, bucket, 0.0000001)
}

func TestBucketUserByIntAttr(t *testing.T) {
	userKey := "userKeyD"
	custom := map[string]interface{}{
		"intAttr": 33333,
	}
	user := User{Key: &userKey, Custom: &custom}
	bucket := bucketUser(user, "hashKey", "intAttr", "saltyA")
	assert.InEpsilon(t, 0.54771423, bucket, 0.0000001)

	custom = map[string]interface{}{
		"stringAttr": "33333",
	}
	user = User{Key: &userKey, Custom: &custom}
	bucket2 := bucketUser(user, "hashKey", "stringAttr", "saltyA")
	assert.InEpsilon(t, bucket, bucket2, 0.0000001)
}

func TestBucketUserByFloatAttrNotAllowed(t *testing.T) {
	userKey := "userKeyE"
	custom := map[string]interface{}{
		"floatAttr": 999.999,
	}
	user := User{Key: &userKey, Custom: &custom}
	bucket := bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InDelta(t, 0.0, bucket, 0.0000001)
}

func booleanFlagWithClause(clause Clause) FeatureFlag {
	return FeatureFlag{
		Key: "feature",
		On:  true,
		Rules: []Rule{
			Rule{Clauses: []Clause{clause}, VariationOrRollout: VariationOrRollout{Variation: intPtr(1)}},
		},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{false, true},
	}
}
