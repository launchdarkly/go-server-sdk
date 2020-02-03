package ldclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
)

var flagUser = lduser.NewUser("x")
var emptyDataStore = newInMemoryDataStoreInternal(Config{})
var fallthroughValue = ldvalue.String("fall")
var offValue = ldvalue.String("off")
var onValue = ldvalue.String("on")

func intPtr(n int) *int {
	return &n
}

func TestFlagReturnsOffVariationIfFlagIsOff(t *testing.T) {
	f := FeatureFlag{
		Key:          "feature",
		On:           false,
		OffVariation: intPtr(1),
		Fallthrough:  variationOrRollout{Variation: intPtr(0)},
		Variations:   []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, offValue, result.JSONValue)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonOff(), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsNilIfFlagIsOffAndOffVariationIsUnspecified(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          false,
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, ldvalue.Null(), result.JSONValue)
	assert.Nil(t, result.VariationIndex)
	assert.Equal(t, newEvalReasonOff(), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsFallthroughIfFlagIsOnAndThereAreNoRules(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{},
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, fallthroughValue, result.JSONValue)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, newEvalReasonFallthrough(), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasTooHighVariation(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{},
		Fallthrough: variationOrRollout{Variation: intPtr(999)},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasNegativeVariation(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{},
		Fallthrough: variationOrRollout{Variation: intPtr(-1)},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasNeitherVariationNorRollout(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{},
		Fallthrough: variationOrRollout{},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsErrorIfFallthroughHasEmptyRolloutVariationList(t *testing.T) {
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{},
		Fallthrough: variationOrRollout{Rollout: &rollout{Variations: []weightedVariation{}}},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsOffVariationIfPrerequisiteIsNotFound(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
	}

	result, events := f0.evaluateDetail(flagUser, emptyDataStore, false)
	assert.Equal(t, offValue, result.JSONValue)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagReturnsOffVariationAndEventIfPrerequisiteIsOff(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           false,
		OffVariation: intPtr(1),
		// note that even though it returns the desired variation, it is still off and therefore not a match
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{ldvalue.String("nogo"), ldvalue.String("go")},
		Version:     2,
	}
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Features, &f1)

	result, events := f0.evaluateDetail(flagUser, dataStore, false)
	assert.Equal(t, offValue, result.JSONValue)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, ldvalue.String("go"), e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, intPtr(1), e.Variation)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestFlagReturnsOffVariationAndEventIfPrerequisiteIsNotMet(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           true,
		OffVariation: intPtr(1),
		Fallthrough:  variationOrRollout{Variation: intPtr(0)},
		Variations:   []ldvalue.Value{ldvalue.String("nogo"), ldvalue.String("go")},
		Version:      2,
	}
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Features, &f1)

	result, events := f0.evaluateDetail(flagUser, dataStore, false)
	assert.Equal(t, offValue, result.JSONValue)
	assert.Equal(t, intPtr(1), result.VariationIndex)
	assert.Equal(t, newEvalReasonPrerequisiteFailed("feature1"), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, ldvalue.String("nogo"), e.Value)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, intPtr(0), e.Variation)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestFlagReturnsFallthroughVariationAndEventIfPrerequisiteIsMetAndThereAreNoRules(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           true,
		OffVariation: intPtr(1),
		Fallthrough:  variationOrRollout{Variation: intPtr(1)}, // this 1 matches the 1 in the prerequisites array
		Variations:   []ldvalue.Value{ldvalue.String("nogo"), ldvalue.String("go")},
		Version:      2,
	}
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Features, &f1)

	result, events := f0.evaluateDetail(flagUser, dataStore, false)
	assert.Equal(t, fallthroughValue, result.JSONValue)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, newEvalReasonFallthrough(), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, ldvalue.String("go"), e.Value)
	assert.Equal(t, intPtr(1), e.Variation)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestPrerequisiteCanMatchWithNonScalarValue(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:          "feature1",
		On:           true,
		OffVariation: intPtr(1),
		Fallthrough:  variationOrRollout{Variation: intPtr(1)}, // this 1 matches the 1 in the prerequisites array
		Variations:   []ldvalue.Value{ldvalue.ArrayOf(ldvalue.String("000")), ldvalue.ArrayOf(ldvalue.String("001"))},
		Version:      2,
	}
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Features, &f1)

	result, events := f0.evaluateDetail(flagUser, dataStore, false)
	assert.Equal(t, fallthroughValue, result.JSONValue)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, newEvalReasonFallthrough(), result.Reason)

	assert.Equal(t, 1, len(events))
	e := events[0]
	assert.Equal(t, f1.Key, e.Key)
	assert.Equal(t, ldvalue.ArrayOf(ldvalue.String("001")), e.Value)
	assert.Equal(t, intPtr(1), e.Variation)
	assert.Equal(t, intPtr(f1.Version), e.Version)
	assert.Equal(t, strPtr(f0.Key), e.PrereqOf)
}

func TestMultipleLevelsOfPrerequisiteProduceMultipleEvents(t *testing.T) {
	f0 := FeatureFlag{
		Key:           "feature0",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature1", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(0)},
		Variations:    []ldvalue.Value{fallthroughValue, offValue, onValue},
		Version:       1,
	}
	f1 := FeatureFlag{
		Key:           "feature1",
		On:            true,
		OffVariation:  intPtr(1),
		Prerequisites: []prerequisite{prerequisite{"feature2", 1}},
		Fallthrough:   variationOrRollout{Variation: intPtr(1)}, // this 1 matches the 1 in the prerequisites array
		Variations:    []ldvalue.Value{ldvalue.String("nogo"), ldvalue.String("go")},
		Version:       2,
	}
	f2 := FeatureFlag{
		Key:         "feature2",
		On:          true,
		Fallthrough: variationOrRollout{Variation: intPtr(1)},
		Variations:  []ldvalue.Value{ldvalue.String("nogo"), ldvalue.String("go")},
		Version:     3,
	}
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Features, &f1)
	dataStore.Upsert(Features, &f2)

	result, events := f0.evaluateDetail(flagUser, dataStore, false)
	assert.Equal(t, fallthroughValue, result.JSONValue)
	assert.Equal(t, intPtr(0), result.VariationIndex)
	assert.Equal(t, newEvalReasonFallthrough(), result.Reason)

	assert.Equal(t, 2, len(events))
	// events are generated recursively, so the deepest level of prerequisite appears first

	e0 := events[0]
	assert.Equal(t, f2.Key, e0.Key)
	assert.Equal(t, ldvalue.String("go"), e0.Value)
	assert.Equal(t, intPtr(1), e0.Variation)
	assert.Equal(t, intPtr(f2.Version), e0.Version)
	assert.Equal(t, strPtr(f1.Key), e0.PrereqOf)

	e1 := events[1]
	assert.Equal(t, f1.Key, e1.Key)
	assert.Equal(t, ldvalue.String("go"), e1.Value)
	assert.Equal(t, intPtr(1), e1.Variation)
	assert.Equal(t, intPtr(f1.Version), e1.Version)
	assert.Equal(t, strPtr(f0.Key), e1.PrereqOf)
}

func TestFlagMatchesUserFromTargets(t *testing.T) {
	f := FeatureFlag{
		Key:          "feature",
		On:           true,
		OffVariation: intPtr(1),
		Targets:      []target{target{[]string{"whoever", "userkey"}, 2}},
		Fallthrough:  variationOrRollout{Variation: intPtr(0)},
		Variations:   []ldvalue.Value{fallthroughValue, offValue, onValue},
	}
	user := lduser.NewUser("userkey")

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, onValue, result.JSONValue)
	assert.Equal(t, intPtr(2), result.VariationIndex)
	assert.Equal(t, newEvalReasonTargetMatch(), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestFlagMatchesUserFromRules(t *testing.T) {
	user := lduser.NewUser("userkey")
	f := makeFlagToMatchUser(user, variationOrRollout{Variation: intPtr(2)})

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, onValue, result.JSONValue)
	assert.Equal(t, intPtr(2), result.VariationIndex)
	assert.Equal(t, newEvalReasonRuleMatch(0, "rule-id"), result.Reason)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithTooHighVariationIndexReturnsMalformedFlagError(t *testing.T) {
	user := lduser.NewUser("userkey")
	f := makeFlagToMatchUser(user, variationOrRollout{Variation: intPtr(999)})

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithNegativeVariationIndexReturnsMalformedFlagError(t *testing.T) {
	user := lduser.NewUser("userkey")
	f := makeFlagToMatchUser(user, variationOrRollout{Variation: intPtr(-1)})

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithNoVariationOrRolloutReturnsMalformedFlagError(t *testing.T) {
	user := lduser.NewUser("userkey")
	f := makeFlagToMatchUser(user, variationOrRollout{})

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestRuleWithRolloutWithEmptyVariationsListReturnsMalformedFlagError(t *testing.T) {
	user := lduser.NewUser("userkey")
	f := makeFlagToMatchUser(user, variationOrRollout{Rollout: &rollout{Variations: []weightedVariation{}}})

	result, events := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, newEvalErrorResult(EvalErrorMalformedFlag), result)
	assert.Equal(t, 0, len(events))
}

func TestClauseCanMatchBuiltInAttribute(t *testing.T) {
	c := clause{
		Attribute: "name",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String("Bob")},
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(true), result.JSONValue)
}

func TestClauseCanMatchCustomAttribute(t *testing.T) {
	c := clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.Int(4)},
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Custom("legs", ldvalue.Int(4)).Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(true), result.JSONValue)
}

func TestClauseReturnsFalseForMissingAttribute(t *testing.T) {
	c := clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.Int(4)},
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(false), result.JSONValue)
}

func TestClauseCanBeNegated(t *testing.T) {
	c := clause{
		Attribute: "name",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String("Bob")},
		Negate:    true,
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(false), result.JSONValue)
}

func TestClauseForMissingAttributeIsFalseEvenIfNegated(t *testing.T) {
	c := clause{
		Attribute: "legs",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.Int(4)},
		Negate:    true,
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(false), result.JSONValue)
}

func TestClauseWithUnknownOperatorDoesNotMatch(t *testing.T) {
	c := clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []ldvalue.Value{ldvalue.String("Bob")},
	}
	f := booleanFlagWithClause(c)
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(false), result.JSONValue)
}

func TestClauseWithUnknownOperatorDoesNotStopSubsequentRuleFromMatching(t *testing.T) {
	badClause := clause{
		Attribute: "name",
		Op:        "doesSomethingUnsupported",
		Values:    []ldvalue.Value{ldvalue.String("Bob")},
	}
	badRule := flagRule{ID: "bad", Clauses: []clause{badClause}, variationOrRollout: variationOrRollout{Variation: intPtr(1)}}
	goodClause := clause{
		Attribute: "name",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String("Bob")},
	}
	goodRule := flagRule{ID: "good", Clauses: []clause{goodClause}, variationOrRollout: variationOrRollout{Variation: intPtr(1)}}
	f := FeatureFlag{
		Key:         "feature",
		On:          true,
		Rules:       []flagRule{badRule, goodRule},
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{ldvalue.Bool(false), ldvalue.Bool(true)},
	}
	user := lduser.NewUserBuilder("key").Name("Bob").Build()

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(true), result.JSONValue)
	assert.Equal(t, newEvalReasonRuleMatch(1, "good"), result.Reason)
}

func TestSegmentMatchClauseRetrievesSegmentFromStore(t *testing.T) {
	segment := Segment{
		Key:      "segkey",
		Included: []string{"foo"},
	}
	clause := clause{Attribute: "", Op: "segmentMatch", Values: []ldvalue.Value{ldvalue.String("segkey")}}
	f := booleanFlagWithClause(clause)
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Segments, &segment)
	user := lduser.NewUser("foo")

	result, _ := f.evaluateDetail(user, dataStore, false)
	assert.Equal(t, ldvalue.Bool(true), result.JSONValue)
}

func TestSegmentMatchClauseFallsThroughIfSegmentNotFound(t *testing.T) {
	clause := clause{Attribute: "", Op: "segmentMatch", Values: []ldvalue.Value{ldvalue.String("segkey")}}
	f := booleanFlagWithClause(clause)
	user := lduser.NewUser("foo")

	result, _ := f.evaluateDetail(user, emptyDataStore, false)
	assert.Equal(t, ldvalue.Bool(false), result.JSONValue)
}

func TestCanMatchJustOneSegmentFromList(t *testing.T) {
	segment := Segment{
		Key:      "segkey",
		Included: []string{"foo"},
	}
	clause := clause{
		Attribute: "",
		Op:        "segmentMatch",
		Values:    []ldvalue.Value{ldvalue.String("unknownsegkey"), ldvalue.String("segkey")},
	}
	f := booleanFlagWithClause(clause)
	dataStore := newInMemoryDataStoreInternal(Config{})
	dataStore.Upsert(Segments, &segment)
	user := lduser.NewUser("foo")

	result, _ := f.evaluateDetail(user, dataStore, false)
	assert.Equal(t, ldvalue.Bool(true), result.JSONValue)
}

func TestVariationIndexForUser(t *testing.T) {
	wv1 := weightedVariation{Variation: 0, Weight: 60000.0}
	wv2 := weightedVariation{Variation: 1, Weight: 40000.0}
	rollout := rollout{Variations: []weightedVariation{wv1, wv2}}
	rule := flagRule{variationOrRollout: variationOrRollout{Rollout: &rollout}}

	variationIndex := rule.variationIndexForUser(lduser.NewUser("userKeyA"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)

	variationIndex = rule.variationIndexForUser(lduser.NewUser("userKeyB"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 1, *variationIndex)

	variationIndex = rule.variationIndexForUser(lduser.NewUser("userKeyC"), "hashKey", "saltyA")
	assert.NotNil(t, variationIndex)
	assert.Equal(t, 0, *variationIndex)
}

func TestBucketUserByKey(t *testing.T) {
	user := lduser.NewUser("userKeyA")
	bucket := bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.42157587, bucket, 0.0000001)

	user = lduser.NewUser("userKeyB")
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.6708485, bucket, 0.0000001)

	user = lduser.NewUser("userKeyC")
	bucket = bucketUser(user, "hashKey", "key", "saltyA")
	assert.InEpsilon(t, 0.10343106, bucket, 0.0000001)
}

func TestBucketUserByIntAttr(t *testing.T) {
	user := lduser.NewUserBuilder("userKeyD").Custom("intAttr", ldvalue.Int(33333)).Build()
	bucket := bucketUser(user, "hashKey", "intAttr", "saltyA")
	assert.InEpsilon(t, 0.54771423, bucket, 0.0000001)

	user = lduser.NewUserBuilder("userKeyD").Custom("stringAttr", ldvalue.String("33333")).Build()
	bucket2 := bucketUser(user, "hashKey", "stringAttr", "saltyA")
	assert.InEpsilon(t, bucket, bucket2, 0.0000001)
}

func TestBucketUserByFloatAttrNotAllowed(t *testing.T) {
	user := lduser.NewUserBuilder("userKeyE").Custom("floatAttr", ldvalue.Float64(999.999)).Build()
	bucket := bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InDelta(t, 0.0, bucket, 0.0000001)
}

func TestBucketUserByFloatAttrThatIsReallyAnIntIsAllowed(t *testing.T) {
	user := lduser.NewUserBuilder("userKeyE").Custom("floatAttr", ldvalue.Float64(33333)).Build()
	bucket := bucketUser(user, "hashKey", "floatAttr", "saltyA")
	assert.InEpsilon(t, 0.54771423, bucket, 0.0000001)
}

func booleanFlagWithClause(c clause) FeatureFlag {
	return FeatureFlag{
		Key: "feature",
		On:  true,
		Rules: []flagRule{
			flagRule{Clauses: []clause{c}, variationOrRollout: variationOrRollout{Variation: intPtr(1)}},
		},
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{ldvalue.Bool(false), ldvalue.Bool(true)},
	}
}

func newEvalErrorResult(kind EvalErrorKind) EvaluationDetail {
	return EvaluationDetail{Reason: newEvalReasonError(kind)}
}

func makeClauseToMatchUser(user lduser.User) clause {
	return clause{
		Attribute: "key",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String(user.GetKey())},
	}
}

func makeClauseToNotMatchUser(user lduser.User) clause {
	return clause{
		Attribute: "key",
		Op:        "in",
		Values:    []ldvalue.Value{ldvalue.String("not-" + user.GetKey())},
	}
}

func makeFlagToMatchUser(user lduser.User, vr variationOrRollout) FeatureFlag {
	return FeatureFlag{
		Key:          "feature",
		On:           true,
		OffVariation: intPtr(1),
		Rules: []flagRule{
			flagRule{
				ID:                 "rule-id",
				Clauses:            []clause{makeClauseToMatchUser(user)},
				variationOrRollout: vr,
			},
		},
		Fallthrough: variationOrRollout{Variation: intPtr(0)},
		Variations:  []ldvalue.Value{fallthroughValue, offValue, onValue},
	}
}
