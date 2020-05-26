package evaluation

import (
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldreason"
	"gopkg.in/launchdarkly/go-sdk-common.v2/lduser"
	"gopkg.in/launchdarkly/go-sdk-common.v2/ldvalue"
	"gopkg.in/launchdarkly/go-server-sdk-evaluation.v1/ldmodel"
)

type evaluator struct {
	dataProvider DataProvider
}

// NewEvaluator creates an Evaluator, specifying a DataProvider that it will use if it needs to
// query additional feature flags or user segments during an evaluation.
func NewEvaluator(dataProvider DataProvider) Evaluator {
	return &evaluator{dataProvider}
}

func (e *evaluator) Evaluate(
	flag ldmodel.FeatureFlag,
	user lduser.User,
	prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder,
) ldreason.EvaluationDetail {
	if !flag.On {
		return getOffValue(&flag, ldreason.NewEvalReasonOff())
	}

	// Note that all of our internal methods operate on pointers (*User, *FeatureFlag, *Clause, etc.);
	// this is done to avoid the overhead of repeatedly copying these structs by value. We know that
	// the pointers cannot be nil, since the entry point is always Evaluate which does receive its
	// parameters by value; mutability is not a concern, since User is immutable and the evaluation
	// code will never modify anything in the data model. Taking the address of these structs will not
	// cause heap escaping because we are never *returning* pointers (and never passing them to
	// external code such as prerequisiteFlagEventRecorder).

	prereqErrorReason, ok := e.checkPrerequisites(&flag, &user, prerequisiteFlagEventRecorder)
	if !ok {
		return getOffValue(&flag, prereqErrorReason)
	}

	key := user.GetKey()

	// Check to see if targets match
	for _, target := range flag.Targets {
		for _, value := range target.Values {
			if value == key {
				return getVariation(&flag, target.Variation, ldreason.NewEvalReasonTargetMatch())
			}
		}
	}

	// Now walk through the rules and see if any match
	for ruleIndex, rule := range flag.Rules {
		r := rule
		if e.ruleMatchesUser(&r, &user) {
			reason := ldreason.NewEvalReasonRuleMatch(ruleIndex, rule.ID)
			return getValueForVariationOrRollout(&flag, rule.VariationOrRollout, &user, reason)
		}
	}

	return getValueForVariationOrRollout(&flag, flag.Fallthrough, &user, ldreason.NewEvalReasonFallthrough())
}

// Returns an empty reason if all prerequisites are OK, otherwise constructs an error reason that describes the failure
func (e *evaluator) checkPrerequisites(
	f *ldmodel.FeatureFlag,
	user *lduser.User,
	prerequisiteFlagEventRecorder PrerequisiteFlagEventRecorder,
) (ldreason.EvaluationReason, bool) {
	if len(f.Prerequisites) == 0 {
		return ldreason.EvaluationReason{}, true
	}

	for _, prereq := range f.Prerequisites {
		prereqFeatureFlag, ok := e.dataProvider.GetFeatureFlag(prereq.Key)
		if !ok {
			return ldreason.NewEvalReasonPrerequisiteFailed(prereq.Key), false
		}
		prereqOK := true

		prereqResult := e.Evaluate(prereqFeatureFlag, *user, prerequisiteFlagEventRecorder)
		if !prereqFeatureFlag.On || prereqResult.IsDefaultValue() || prereqResult.VariationIndex != prereq.Variation {
			// Note that if the prerequisite flag is off, we don't consider it a match no matter what its
			// off variation was. But we still need to evaluate it in order to generate an event.
			prereqOK = false
		}

		if prerequisiteFlagEventRecorder != nil {
			event := PrerequisiteFlagEvent{f.Key, *user, prereqFeatureFlag, prereqResult}
			prerequisiteFlagEventRecorder(event)
		}

		if !prereqOK {
			return ldreason.NewEvalReasonPrerequisiteFailed(prereq.Key), false
		}
	}
	return ldreason.EvaluationReason{}, true
}

func getVariation(f *ldmodel.FeatureFlag, index int, reason ldreason.EvaluationReason) ldreason.EvaluationDetail {
	if index < 0 || index >= len(f.Variations) {
		return ldreason.NewEvaluationDetailForError(ldreason.EvalErrorMalformedFlag, ldvalue.Null())
	}
	return ldreason.NewEvaluationDetail(f.Variations[index], index, reason)
}

func getOffValue(f *ldmodel.FeatureFlag, reason ldreason.EvaluationReason) ldreason.EvaluationDetail {
	if f.OffVariation == nil {
		return ldreason.NewEvaluationDetail(ldvalue.Null(), -1, reason)
	}
	return getVariation(f, *f.OffVariation, reason)
}

func getValueForVariationOrRollout(
	f *ldmodel.FeatureFlag,
	vr ldmodel.VariationOrRollout,
	user *lduser.User,
	reason ldreason.EvaluationReason,
) ldreason.EvaluationDetail {
	index := variationIndexForUser(vr, user, f.Key, f.Salt)
	if index == nil {
		return ldreason.NewEvaluationDetailForError(ldreason.EvalErrorMalformedFlag, ldvalue.Null())
	}
	return getVariation(f, *index, reason)
}

func (e *evaluator) ruleMatchesUser(rule *ldmodel.FlagRule, user *lduser.User) bool {
	for _, clause := range rule.Clauses {
		c := clause
		if !e.clauseMatchesUser(&c, user) {
			return false
		}
	}
	return true
}

func clauseMatchesUserNoSegments(clause *ldmodel.Clause, user *lduser.User) bool {
	uValue := user.GetAttribute(clause.Attribute)
	if uValue.IsNull() {
		return false
	}
	matchFn := operatorFn(clause.Op)

	// If the user value is an array, see if the intersection is non-empty. If so, this clause matches
	if uValue.Type() == ldvalue.ArrayType {
		for i := 0; i < uValue.Count(); i++ {
			if matchAny(matchFn, uValue.GetByIndex(i), clause.Values) {
				return maybeNegate(clause, true)
			}
		}
		return maybeNegate(clause, false)
	}

	return maybeNegate(clause, matchAny(matchFn, uValue, clause.Values))
}

func (e *evaluator) clauseMatchesUser(clause *ldmodel.Clause, user *lduser.User) bool {
	// In the case of a segment match operator, we check if the user is in any of the segments,
	// and possibly negate
	if clause.Op == ldmodel.OperatorSegmentMatch {
		for _, value := range clause.Values {
			if value.Type() == ldvalue.StringType {
				if segment, segmentOk := e.dataProvider.GetSegment(value.StringValue()); segmentOk {
					if matches, _ := segmentContainsUser(segment, user); matches {
						return maybeNegate(clause, true)
					}
				}
			}
		}
		return maybeNegate(clause, false)
	}

	return clauseMatchesUserNoSegments(clause, user)
}

func maybeNegate(clause *ldmodel.Clause, b bool) bool {
	if clause.Negate {
		return !b
	}
	return b
}

func matchAny(fn opFn, value ldvalue.Value, values []ldvalue.Value) bool {
	for _, v := range values {
		if fn(value, v) {
			return true
		}
	}
	return false
}

func variationIndexForUser(r ldmodel.VariationOrRollout, user *lduser.User, key, salt string) *int {
	if r.Variation != nil {
		return r.Variation
	}
	if r.Rollout == nil {
		// This is an error (malformed flag); either Variation or Rollout must be non-nil.
		return nil
	}

	bucketBy := lduser.KeyAttribute
	if r.Rollout.BucketBy != nil {
		bucketBy = *r.Rollout.BucketBy
	}

	var bucket = bucketUser(user, key, bucketBy, salt)
	var sum float32

	if len(r.Rollout.Variations) == 0 {
		// This is an error (malformed flag); there must be at least one weighted variation.
		return nil
	}
	for _, wv := range r.Rollout.Variations {
		sum += float32(wv.Weight) / 100000.0
		if bucket < sum {
			return &wv.Variation
		}
	}
	// If we get here, it's due to either a rounding error or weights that don't add up to 100000
	return nil
}
