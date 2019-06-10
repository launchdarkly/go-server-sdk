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

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, "off", result.Value)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, evalReasonOffInstance, result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsNilIfFlagIsOffAndOffVariationIsUnspecified(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          false,
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Nil(t, result.Value)
	assert.Nil(t, result.VariationIndex)
	assert.Equal(t, evalReasonOffInstance, result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsFallthroughIfFlagIsOnAndThereAreNoRules(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, "fall", result.Value)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasTooHighVariation(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{},
		Fallthrough: VariationOrRollout{Variation: intPtr(999)},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasNegativeVariation(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{},
		Fallthrough: VariationOrRollout{Variation: intPtr(-1)},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasNeitherVariationNorRollout(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{},
		Fallthrough: VariationOrRollout{},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasEmptyRolloutVariationList(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{},
		Fallthrough: VariationOrRollout{Rollout: &Rollout{Variations: []WeightedVariation{}}},
		Variations:  []interface{}{"fall", "off", "on"},
	}

	result, events := f.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
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

	result, events := f0.EvaluateDetail(flagUser, emptyFeatureStore, false)
	assert.Equal(t, "off", result.Value)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsOffVariationAndEventIfPrerequisiteIsOff(t *testing.T) {
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
		On:           false,
		OffVariation: intPtr(1),
		// note that even though it returns the desired variation, it is still off and therefore not a match
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"nogo", "go"},
		Version:     2,
	}
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Features, &f1)

	result, events := f0.EvaluateDetail(flagUser, featureStore, false)
	assert.Equal(t, "off", result.Value)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, "go", e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, intPtr(1), e.Variation)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
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

	result, events := f0.EvaluateDetail(flagUser, featureStore, false)
	assert.Equal(t, "off", result.Value)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, "nogo", e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, intPtr(0), e.Variation)
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

	result, events := f0.EvaluateDetail(flagUser, featureStore, false)
	assert.Equal(t, "fall", result.Value)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, "go", e.Value)
	assert.Equal(t, intPtr(1), e.Variation)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestPrerequisiteCanMatchWithNonScalarValue(t *testing.T) {
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
		Variations:   []interface{}{[]interface{}{"000"}, []interface{}{"001"}},
		Version:      2,
	}
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Features, &f1)

	result, events := f0.EvaluateDetail(flagUser, featureStore, false)
	assert.Equal(t, "fall", result.Value)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, []interface{}{"001"}, e.Value)
	assert.Equal(t, intPtr(1), e.Variation)
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

	result, events := f0.EvaluateDetail(flagUser, featureStore, false)
	assert.Equal(t, "fall", result.Value)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, evalReasonFallthroughInstance, result.Reason)

	assert.Equal(t, 2, len(events))
	// events are generated recursively, so the deepest level of prerequisite appears first

	e0 := events[0]
	assert.Equal(t, f2.Key, e0.Key)
	assert.Equal(t, "go", e0.Value)
	assert.Equal(t, intPtr(1), e0.Variation)
	assert.Equal(t, intPtr(f2.Version), e0.Version)
	assert.Equal(t, strPtr(f1.Key), e0.PrereqOf)

	e1 := events[1]
	assert.Equal(t, f1.Key, e1.Key)
	assert.Equal(t, "go", e1.Value)
	assert.Equal(t, intPtr(1), e1.Variation)
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

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, "on", result.Value)
	assert.Equal(t, intPtr(2), result.VariationIndex)
	assert.Equal(t, evalReasonTargetMatchInstance, result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagMatchesUserFromRules(t *testing.T) {
	user := NewUser("userkey")
	f := makeFlagToMatchUser(user, VariationOrRollout{Variation: intPtr(2)})

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, "on", result.Value)
	assert.Equal(t, intPtr(2), result.VariationIndex)
	assert.Equal(t, newEvalReasonRuleMatch(0, "rule-id"), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithTooHighVariationIndexReturnsMalformedFlagError(t *testing.T) {
	user := NewUser("userkey")
	f := makeFlagToMatchUser(user, VariationOrRollout{Variation: intPtr(999)})

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithNegativeVariationIndexReturnsMalformedFlagError(t *testing.T) {
	user := NewUser("userkey")
	f := makeFlagToMatchUser(user, VariationOrRollout{Variation: intPtr(-1)})

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithNoVariationOrRolloutReturnsMalformedFlagError(t *testing.T) {
	user := NewUser("userkey")
	f := makeFlagToMatchUser(user, VariationOrRollout{})

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithRolloutWithEmptyVariationsListReturnsMalformedFlagError(t *testing.T) {
	user := NewUser("userkey")
	f := makeFlagToMatchUser(user, VariationOrRollout{Rollout: &Rollout{Variations: []WeightedVariation{}}})

	result, events := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
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

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, true, result.Value)
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

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, true, result.Value)
}

func TestClauseReturnsFalseForMissingAttribute(t *testing.T) {
	clause := Clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []interface{}{4},
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, false, result.Value)
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

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, false, result.Value)
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

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, false, result.Value)
}

func TestClauseWithUnknownOperatorDoesNotMatch(t *testing.T) {
	clause := Clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []interface{}{"Bob"},
	}
	f := booleanFlagWithClause(clause)
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, false, result.Value)
}

func TestClauseWithUnknownOperatorDoesNotStopSubsequentRuleFromMatching(t *testing.T) {
	badClause := Clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []interface{}{"Bob"},
	}
	badRule := Rule{ID: "bad", Clauses: []Clause{badClause}, VariationOrRollout: VariationOrRollout{Variation: intPtr(1)}}
	goodClause := Clause{
		Attribute: "name",
		Op:        "in",
		Values:    []interface{}{"Bob"},
	}
	goodRule := Rule{ID: "good", Clauses: []Clause{goodClause}, VariationOrRollout: VariationOrRollout{Variation: intPtr(1)}}
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []Rule{badRule, goodRule},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{false, true},
	}
	user := User{Key: strPtr("key"), Name: strPtr("Bob")}

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, true, result.Value)
	assert.Equal(t, newEvalReasonRuleMatch(1, "good"), result.Reason)
}

func TestSegmentMatchClauseRetrievesSegmentFromStore(t *testing.T) {
	segment := Segment{
		Key:      "segkey",
		Included: []string{"foo"},
	}
	clause := Clause{Attribute: "", Op: "segmentMatch", Values: []interface{}{"segkey"}}
	f := booleanFlagWithClause(clause)
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Segments, &segment)
	user := NewUser("foo")

	result, _ := f.EvaluateDetail(user, featureStore, false)
	assert.Equal(t, true, result.Value)
}

func TestSegmentMatchClauseFallsThroughIfSegmentNotFound(t *testing.T) {
	clause := Clause{Attribute: "", Op: "segmentMatch", Values: []interface{}{"segkey"}}
	f := booleanFlagWithClause(clause)
	user := NewUser("foo")

	result, _ := f.EvaluateDetail(user, emptyFeatureStore, false)
	assert.Equal(t, false, result.Value)
}

func TestCanMatchJustOneSegmentFromList(t *testing.T) {
	segment := Segment{
		Key:      "segkey",
		Included: []string{"foo"},
	}
	clause := Clause{Attribute: "", Op: "segmentMatch", Values: []interface{}{"unknownsegkey", "segkey"}}
	f := booleanFlagWithClause(clause)
	featureStore := NewInMemoryFeatureStore(nil)
	featureStore.Upsert(Segments, &segment)
	user := NewUser("foo")

	result, _ := f.EvaluateDetail(user, featureStore, false)
	assert.Equal(t, true, result.Value)
}

func TestVariationIndexForUser(t *testing.T) {
	wv1 := WeightedVariation{Variation: 0, Weight: 60000.0}
	wv2 := WeightedVariation{Variation: 1, Weight: 40000.0}
	rollout := Rollout{Variations: []WeightedVariation{wv1, wv2}}
	rule := Rule{VariationOrRollout: VariationOrRollout{Rollout: &rollout}}

	variationIndex := rule.variationIndexForUser(NewUser("userKeyA"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)

	variationIndex = rule.variationIndexForUser(NewUser("userKeyB"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 1, *variationIndex)

	variationIndex = rule.variationIndexForUser(NewUser("userKeyC"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)
}

func TestBucketUserByKey(t *testing.T) {
	user := NewUser("userKeyA")
	bucket := bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.42157587, bucket, 0.0000001)

	user = NewUser("userKeyB")
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.6708485, bucket, 0.0000001)

	user = NewUser("userKeyC")
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
		"floatAttr": float64(999.999),
	}
	user := User{Key: &userKey, Custom: &custom}
	bucket := bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InDelta(t, 0.0, bucket, 0.0000001)
}

func TestBucketUserByFloatAttrThatIsReallyAnIntIsAllowed(t *testing.T) {
	userKey := "userKeyE"
	custom := map[string]interface{}{
		"floatAttr": float64(33333),
	}
	user := User{Key: &userKey, Custom: &custom}
	bucket := bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InEpsilon(t, 0.54771423, bucket, 0.0000001)
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

func newEvalErrorResult(kind EvalErrorKind) EvaluationDetail {
	return EvaluationDetail{Reason: newEvalReasonError(kind)}
}

func makeFlagToMatchUser(user User, variationOrRollout VariationOrRollout) FeatureFlag {
	clause := Clause{
		Attribute: "key",
		Op:        "in",
		Values:    []interface{}{*user.Key},
	}
	return FeatureFlag{
		Key:          "feature",
		On:           true,
		OffVariation: intPtr(1),
		Rules: []Rule{
			Rule{
				ID:                 "rule-id",
				Clauses:            []Clause{clause},
				VariationOrRollout: variationOrRollout,
			},
		},
		Fallthrough: VariationOrRollout{Variation: intPtr(0)},
		Variations:  []interface{}{"fall", "off", "on"},
	}
}
